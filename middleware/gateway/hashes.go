package gateway

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/socket"
)

const (
	MaxActionDelay  = 100
	ReservationTime = 5
	sent            = 0
	sealed          = 1
	commit          = 2
)

type Propose struct {
	data []byte
	conn *socket.SignedConnection
}

type Pending struct {
	action []byte
	hash   crypto.Hash
	origin *socket.SignedConnection
}

type Sealed struct {
	action []byte
}

type Seal struct {
	Epoch  uint64
	Origin *socket.SignedConnection
}

type SealOnBlock struct {
	Epoch     uint64
	BlockHash crypto.Hash
	Action    []byte
}

type PendingUpdate struct {
	hash   crypto.Hash
	action byte
}

type ActionVault struct {
	// clock is the current epoch as perceived by the vault
	// an external process is responsible for updating the clock
	clock uint64
	// all actions received and not yet incorporated into a commit block
	pending map[crypto.Hash]*Pending
	// all pending actions bucketed by epoch
	epoch [][]crypto.Hash
	// reseved bucketed by reservation epoch
	sent [ReservationTime]map[crypto.Hash]*Pending
	// sealed but not commit actions
	sealed map[crypto.Hash]Seal
	// commited actions
	committed []map[crypto.Hash]struct{}
	// channel to mark actions as sealed
	seal chan SealOnBlock
	// channel to mark actions as commit
	commit chan crypto.Hash
	// channel to pop actions from the vault
	Pop chan []byte
	// channel to push new actions into the vault
	Push chan *Propose
	// channel to trigger a clock update
	timer chan struct{}
}

func NewActionVault(ctx context.Context, epoch uint64, actions chan *Propose) *ActionVault {
	vault := ActionVault{
		clock:   epoch,
		pending: make(map[crypto.Hash]*Pending),
		epoch:   make([][]crypto.Hash, 0),
		seal:    make(chan SealOnBlock),
		sealed:  make(map[crypto.Hash]Seal),
		Pop:     make(chan []byte, 1),
		Push:    actions,
		timer:   make(chan struct{}),
	}
	for n := 0; n < 2*MaxActionDelay; n++ {
		vault.epoch = append(vault.epoch, make([]crypto.Hash, 0))
		vault.committed = append(vault.committed, make(map[crypto.Hash]struct{}))
	}
	for n := 0; n < ReservationTime; n++ {
		vault.sent[n] = make(map[crypto.Hash]*Pending)
	}

	go func() {
		done := ctx.Done()
		defer func() {
			close(vault.Pop)
		}()
		for {

			if len(vault.pending) == 0 {
				// if there is no action to pop
				select {
				case <-done:
					return
				case <-vault.timer:
					vault.movenext()
				case data, ok := <-vault.Push:
					if !ok {
						return
					}
					if !vault.push(data) {
						data.conn.Send([]byte{messages.MsgError})
					}
				case sealed := <-vault.seal:
					vault.sealAction(sealed.Action, sealed.Epoch, sealed.BlockHash)
				case hash := <-vault.commit:
					vault.commitAction(hash)
				}
			} else {
				// if there are actions to pop
				select {
				case <-done:
					return
				case <-vault.timer:
					vault.movenext()
				case vault.Pop <- vault.pop():
				case data, ok := <-vault.Push:
					if !ok {
						return
					}
					if !vault.push(data) {
						data.conn.Send([]byte{messages.MsgError})
					}
				case sealed := <-vault.seal:
					vault.sealAction(sealed.Action, sealed.Epoch, sealed.BlockHash)
				case hash := <-vault.commit:
					vault.commitAction(hash)
				}
			}
		}
	}()
	return &vault
}

// NextEpoch sends a clock update request to the vault
func (v *ActionVault) NextEpoch() {
	v.timer <- struct{}{}
}

func (v *ActionVault) movenext() {
	v.clock += 1
	// liberates reserved actions not yet sealed
	for _, pending := range v.sent[0] {
		v.pending[pending.hash] = pending
	}
	for n := 0; n < ReservationTime-1; n++ {
		v.sent[n] = v.sent[n+1]
	}
	v.sent[ReservationTime-1] = make(map[crypto.Hash]*Pending)
	if v.clock > MaxActionDelay {
		v.epoch = append(v.epoch[1:], make([]crypto.Hash, 0))
		v.committed = append(v.committed[1:], make(map[crypto.Hash]struct{}))
	} else {
		v.epoch = append(v.epoch, make([]crypto.Hash, 0))
	}
}

func (v *ActionVault) sealAction(action []byte, blockEpoch uint64, blockHash crypto.Hash) {
	hash := crypto.Hasher(action)
	seal := Seal{
		Epoch: actions.GetEpochFromByteArray(action),
	}
	var pending *Pending
	hasPending := false
	for r := 0; r < ReservationTime; r++ {
		if pending, hasPending = v.sent[r][hash]; hasPending {
			break
		}
	}
	if !hasPending {
		v.sealed[hash] = seal
		return
	}
	seal.Origin = pending.origin
	v.sealed[hash] = seal
	delete(v.pending, hash)
	for r := 0; r < ReservationTime; r++ {
		delete(v.sent[r], hash)
	}
	msg := messages.SealedAction(hash, blockEpoch, blockHash)
	pending.origin.Send(msg)
}

func (v *ActionVault) commitAction(hash crypto.Hash) {
	seal, ok := v.sealed[hash]
	if !ok {
		slog.Warn("ActionStore: commit called on unsealed action", "hash", hash)
		return
	}
	delete(v.sealed, hash)
	bucket := v.bucket(seal.Epoch)
	if bucket < 0 || bucket >= ReservationTime {
		slog.Error("ActionStore: commit called on unreserved action", "hash", hash)
		return
	}
	v.committed[bucket][hash] = struct{}{}
	if seal.Origin != nil {
		msg := append([]byte{messages.MsgActionCommit}, hash[:]...)
		seal.Origin.Send(msg)
	}
}

func (v *ActionVault) sentAction(pending *Pending) {
	if pending == nil {
		slog.Warn("ActionStore: sent called on unkown action")
		return
	}
	delete(v.pending, pending.hash)
	// mark as sent
	v.sent[ReservationTime-1][pending.hash] = pending
	// inform the connection of status
	msg := append([]byte{messages.MsgActionForward}, pending.hash[:]...)
	if pending.origin != nil {
		pending.origin.Send(msg)
	}
}

func (v *ActionVault) pop() []byte {
	if len(v.pending) == 0 {
		return nil
	}
	for n, hashesByEpoch := range v.epoch {
		if len(hashesByEpoch) > 0 {
			// cleanedHashes is used to purge epoch hashes from actions
			// already sealed and thus not pending anymore
			cleanedHashes := make([]crypto.Hash, 0, len(hashesByEpoch))
			for m, hash := range hashesByEpoch {
				if pend, ok := v.pending[hash]; ok {
					for r := 0; r < ReservationTime; r++ {
						// check if the message is not marked as sent
						if _, ok := v.sent[r][hash]; !ok {
							// update cleaned hashes up to this point
							v.epoch[n] = append(cleanedHashes, hashesByEpoch[m:]...)
							// update vault state
							v.sentAction(pend)
							return pend.action
						} else {
							// keep pending message on epoch hashed
							cleanedHashes = append(cleanedHashes, hash)
						}
					}
				}
			}
			v.epoch[n] = cleanedHashes
		}
	}
	return nil
}

func (v *ActionVault) bucket(epoch uint64) int {
	// first epoch in the bucket
	firstBucketEpoch := 0
	if v.clock > MaxActionDelay {
		firstBucketEpoch = int(epoch) - MaxActionDelay
	}
	return int(epoch) - firstBucketEpoch
}

func (v *ActionVault) push(propose *Propose) bool {
	if propose == nil || len(propose.data) == 0 {
		return false // invalid
	}
	action := actions.ParseAction(propose.data)
	if action == nil {
		return false // invalid
	}
	epoch := action.Epoch()
	firstEpoch := uint64(1)
	if v.clock > MaxActionDelay {
		firstEpoch = v.clock - MaxActionDelay
	}
	if epoch == 0 || epoch < firstEpoch || epoch > v.clock+MaxActionDelay {
		return false // reject by epoch out of scope
	}
	if !v.isNew(crypto.Hasher(propose.data), epoch) {
		return true // already in the vault
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
		return false // this should not happen
	}
	v.pending[pending.hash] = &pending
	v.epoch[bucket] = append(v.epoch[bucket], pending.hash)
	return true
}

func (v *ActionVault) isNew(hash crypto.Hash, epoch uint64) bool {
	if _, ok := v.pending[hash]; ok {
		return false
	}
	for r := 0; r < ReservationTime; r++ {
		if _, ok := v.sent[r][hash]; ok {
			return false
		}
	}
	if _, ok := v.sealed[hash]; ok {
		return false
	}
	bucket := v.bucket(epoch)
	if _, ok := v.committed[bucket][hash]; ok {
		return false
	}
	return true
}
