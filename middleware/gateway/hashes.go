package gateway

import (
	"context"
	"log/slog"

	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/socket"
)

const (
	MaxActionDelay = 100
	mark           = 0
	exclude        = 1
	unmark         = 2
)

type Propose struct {
	data     []byte
	conn     *socket.SignedConnection
	response chan bool
}

type Pending struct {
	action []byte
	hash   crypto.Hash
	origin *socket.SignedConnection
}

type PendingUpdate struct {
	hash   crypto.Hash
	action byte
}

type ActionVault struct {
	clock    uint64
	timer    *time.Timer
	pending  map[crypto.Hash]Pending
	epoch    [][]crypto.Hash
	reserved map[crypto.Hash]struct{}
	update   chan PendingUpdate
	Pop      chan *Pending
	Push     chan *Propose
	sync     ClockSync
}

type ClockSync struct {
	Epoch    uint64
	At       time.Time
	Interval time.Duration
}

func (v *ActionVault) pendingUpdate(update PendingUpdate) {
	if update.action == mark {
		if _, ok := v.pending[update.hash]; ok {
			v.reserved[update.hash] = struct{}{}
		}
	} else if update.action == unmark {
		delete(v.reserved, update.hash)
	} else if update.action == exclude {
		delete(v.reserved, update.hash)
		delete(v.pending, update.hash)
	}
}

func (v *ActionVault) pop() *Pending {
	if len(v.pending) == 0 {
		return nil
	}
	for n, hashes := range v.epoch {
		if len(hashes) > 0 {
			cleaned := make([]crypto.Hash, 0, len(hashes))
			for m, hash := range hashes {
				if pend, ok := v.pending[hash]; ok {
					if _, ok := v.reserved[hash]; !ok {
						v.epoch[n] = append(cleaned, hashes[m:]...)
						delete(v.pending, hash)
						return &pend
					}
				} else {
					cleaned = append(cleaned, hash)
				}
			}
		}
	}
	return nil
}

func (v *ActionVault) push(propose *Propose) bool {
	if propose == nil || len(propose.data) == 0 {
		return false
	}
	action := actions.ParseAction(propose.data)
	if action == nil {
		return false
	}
	epoch := action.Epoch()
	if epoch == 0 || epoch < v.clock-MaxActionDelay || epoch > v.clock+MaxActionDelay {
		return false
	}
	pending := Pending{
		action: propose.data,
		hash:   crypto.Hasher(propose.data),
		origin: propose.conn,
	}
	firstBucketEpoch := 0
	if v.clock > MaxActionDelay {
		firstBucketEpoch = int(v.clock) - MaxActionDelay
	}
	bucket := int(epoch) - firstBucketEpoch
	if bucket < 0 || bucket > 2*MaxActionDelay {
		slog.Error("ActionStore: bucket out of range", "bucket", bucket, "epoch", epoch, "current", v.clock)
		return false
	}
	v.pending[pending.hash] = pending
	v.epoch[bucket] = append(v.epoch[bucket], pending.hash)
	return true
}

func (v *ActionVault) reset() {
	v.clock += 1
	nextEpoch := v.sync.At.Add((time.Duration(v.clock) + 1) * v.sync.Interval)
	v.timer.Reset(time.Until(nextEpoch))
}

func NewActionVault(ctx context.Context, sync ClockSync, actions chan *Propose) {
	epochs := uint64(time.Since(sync.At) / sync.Interval)
	nextEpoch := sync.At.Add((time.Duration(epochs) + 1) * sync.Interval)
	timer := time.NewTimer(time.Until(nextEpoch))
	vault := ActionVault{
		clock:    sync.Epoch + epochs,
		pending:  make(map[crypto.Hash]Pending),
		epoch:    make([][]crypto.Hash, 0),
		reserved: make(map[crypto.Hash]struct{}),
		update:   make(chan PendingUpdate),
		Pop:      make(chan *Pending),
		Push:     actions,
		sync:     sync,
	}

	go func() {
		done := ctx.Done()
		defer func() {
			close(vault.Pop)
			timer.Stop()
		}()
		for {

			if len(vault.pending) <= len(vault.reserved) {
				// if there is no action to pop
				select {
				case <-done:
					return
				case <-timer.C:
					vault.reset()

				case data, ok := <-vault.Push:
					if !ok {
						return
					}
					data.response <- vault.push(data)
				case update := <-vault.update:
					vault.pendingUpdate(update)
				}
			} else {
				// if there are actions to pop
				select {
				case <-done:
					return
				case <-timer.C:
					vault.reset()
				case vault.Pop <- vault.pop():
				case data, ok := <-vault.Push:
					if !ok {
						return
					}
					data.response <- vault.push(data)
				case update := <-vault.update:
					vault.pendingUpdate(update)
				case vault.Pop <- vault.pop():
				}
			}
		}
	}()

}
