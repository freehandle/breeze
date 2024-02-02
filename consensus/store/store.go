/*
Package store implements a store for actions.

The store is a FIFO queue of actions ordered by epoch. Actions from oldest
epochs are served first. Actions from epochs older than MaxActionDelay are
discarded.

The store is used by the consensus engine to store actions received from the
gateway. The consensus engine reads actions from the store and sends them to
the swell engine. The store is also used by the swell engine to store actions
received from the consensus engine. The swell engine reads actions from the
store and sends them to the block engine.

store has an internal clock with epoch which should be updated by the validating
node engine.
*/
package store

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
)

const (
	MaxActionDelay = 100 // messages outside current epoch +/- MaxActionDelay are discarded
	mark           = 0
	exclude        = 1
	unmark         = 2
)

// ActionStore is a store for actions. It is a FIFO queue of actions ordered by
// epoch. Actions from oldest epochs are served first. Actions from epochs older
// than MaxActionDelay are discarded.
// Users should use Push and Pop channels to push and pop actions from the store.
// Users should use Mark, Unmark and Exclude methods  to mark, unmark and exclude
// actions.
// Users should use Epoch channel to update the store epoch.
type ActionStore struct {
	clock    uint64
	Live     bool
	epoch    [][]crypto.Hash
	data     map[crypto.Hash]*StoredAction
	reserved map[crypto.Hash]*StoredAction
	Pop      chan *StoredAction
	Push     chan []byte
	evolve   chan struct{}
	updates  chan hashaction
}

type StoredAction struct {
	Epoch  uint64
	Action actions.Action
	Data   []byte
}

// hashaction is used to mark, unmark or exclude an action from the store
// hash is the hash of the action bytes and action is one of mark, unmark or
// exclude.
type hashaction struct {
	hash   crypto.Hash
	action byte
}

// NewActionStore returns a new action store clocked for the specified epoch.
// Users should use Push and Pop channels to push and pop actions from the store.
// Users should use Mark, Unmark and Exclude methods  to mark, unmark and exclude
// actions.
// Users should use Epoch channel to update the store epoch.
func NewActionStore(ctx context.Context, epoch uint64, actions chan []byte) *ActionStore {
	if actions == nil {
		actions = make(chan []byte)
	}
	store := &ActionStore{
		Live:     true,
		data:     make(map[crypto.Hash]*StoredAction),
		reserved: make(map[crypto.Hash]*StoredAction),
		epoch:    make([][]crypto.Hash, 2*MaxActionDelay+1),
		Pop:      make(chan *StoredAction),
		Push:     actions,
		evolve:   make(chan struct{}),
		updates:  make(chan hashaction),
	}
	for n := 0; n < len(store.epoch); n++ {
		store.epoch[n] = make([]crypto.Hash, 0)
	}

	go func() {
		defer func() {
			store.Live = false
			close(store.Pop)
			close(store.Push)
			close(store.evolve)
		}()
		done := ctx.Done()
		for {
			// if store is empty wait for new action
			if len(store.data) == 0 {
				select {
				case <-done:
					return
				case <-store.evolve:
					store.moveNext()
				case data := <-store.Push:
					if len(data) > 0 {
						store.push(data)
					}
				case update := <-store.updates:
					store.update(update)
				}
			} else {
				select {
				case <-done:
					return
				case <-store.evolve:
					store.moveNext()
				case store.Pop <- store.pop():
				case data := <-store.Push:
					if len(data) > 0 {
						store.push(data)
					}
				case update := <-store.updates:
					store.update(update)
				}
			}
		}
	}()
	return store
}

func (a *ActionStore) update(update hashaction) {
	if update.action == mark {
		if stored, ok := a.data[update.hash]; ok {
			a.reserved[update.hash] = stored
			delete(a.data, update.hash)
		}
	} else if update.action == unmark {
		if reserved, ok := a.reserved[update.hash]; ok {
			a.data[update.hash] = reserved
			delete(a.reserved, update.hash)
		}
	} else if update.action == exclude {
		delete(a.reserved, update.hash)
		delete(a.data, update.hash)
	}
}

// Mark marks an action as reserved. Marks temporarily exclude the action with
// associated hash from the pool of available actions.
func (a *ActionStore) Mark(hash crypto.Hash) {
	a.updates <- hashaction{hash, mark}
}

// Unmark unmarks an action as reserved.
func (a *ActionStore) Unmark(hash crypto.Hash) {
	a.updates <- hashaction{hash, unmark}
}

// Exclude permanently deletes an action from the store.
func (a *ActionStore) Exlude(hash crypto.Hash) {
	a.updates <- hashaction{hash, exclude}
}

// pop implements the pop operation of the store. The Pop request is sent
// to the Pop channel. pop returns nil if the store is empty. otherwise it
// returns the oldest action in the store (ordered by epoch and order of arrival).
func (a *ActionStore) pop() *StoredAction {
	if len(a.data) == 0 {
		slog.Error("ActionStore: pop from empty store")
		return nil
	}
	for n, hashes := range a.epoch {
		if len(hashes) > 0 {
			// will purge epoch[n] of excluded actions until it finds an available
			// non reserved action for the epoch
			cleaned := make([]crypto.Hash, 0, len(hashes))
			for m, hash := range hashes {
				if action, ok := a.data[hash]; ok {
					delete(a.data, hash)
					a.reserved[hash] = action
					a.epoch[n] = append(cleaned, hashes[m:]...)
					return action
				} else if _, ok := a.reserved[hash]; ok {
					cleaned = append(cleaned, hash)
				}
			}
		}
	}
	return nil
}

// push implements the push operation of the store. The Push request is sent
// to the Push channel. push adds the action to the store if the action epoch
// is within the MaxActionDelay range of the current epoch.
func (a *ActionStore) push(data []byte) {
	hash := crypto.Hasher(data)
	if _, ok := a.data[hash]; ok {
		return
	}
	if _, ok := a.reserved[hash]; ok {
		return
	}
	epoch := actions.GetEpochFromByteArray(data)
	if epoch == 0 || epoch+MaxActionDelay < a.clock || epoch > a.clock+MaxActionDelay {
		return
	}
	action := actions.ParseAction(data)
	if action == nil {
		return
	}
	firstBucketEpoch := 0
	if a.clock > MaxActionDelay {
		firstBucketEpoch = int(a.clock) - MaxActionDelay
	}
	bucket := int(epoch) - firstBucketEpoch
	if bucket < 0 || bucket > 2*MaxActionDelay {
		slog.Error("ActionStore: bucket out of range", "bucket", bucket, "epoch", epoch, "current", a.clock)
		return
	}
	a.epoch[bucket] = append(a.epoch[bucket], hash)
	a.data[hash] = &StoredAction{
		Epoch:  epoch,
		Action: action,
		Data:   data,
	}
}

// moveNext moves the store to the next epoch. It deletes all actions from
// epochs older than MaxActionDelay.
func (a *ActionStore) moveNext() {
	a.clock += 1
	if a.clock > MaxActionDelay {
		for _, hash := range a.epoch[0] {
			delete(a.data, hash)
		}
		a.epoch = append(a.epoch[1:], make([]crypto.Hash, 0))
	}
}

func (a *ActionStore) NextEpoch() {
	a.evolve <- struct{}{}
}
