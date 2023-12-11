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
	currentEpoch uint64
	Live         bool
	epoch        [][]crypto.Hash
	data         map[crypto.Hash][]byte
	reserved     map[crypto.Hash]struct{}
	Pop          chan []byte
	Push         chan []byte
	Epoch        chan uint64
	hashes       chan hashaction
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
func NewActionStore(ctx context.Context, epoch uint64) *ActionStore {
	store := &ActionStore{
		Live:     true,
		data:     make(map[crypto.Hash][]byte),
		reserved: make(map[crypto.Hash]struct{}),
		epoch:    make([][]crypto.Hash, 2*MaxActionDelay+1),
		Pop:      make(chan []byte),
		Push:     make(chan []byte),
		Epoch:    make(chan uint64),
		hashes:   make(chan hashaction),
	}
	for n := 0; n < len(store.epoch); n++ {
		store.epoch[n] = make([]crypto.Hash, 0)
	}
	go func() {
		defer func() {
			store.Live = false
			close(store.Pop)
			close(store.Push)
			close(store.Epoch)
		}()
		done := ctx.Done()
		for {
			// if store is empty wait for new action
			if len(store.data) == 0 {
				select {
				case <-done:
					return
				case epoch, ok := <-store.Epoch:
					if !ok {
						return
					}
					store.moveNext(epoch)
				case data, ok := <-store.Push:
					if !ok {
						return
					}
					if len(data) > 0 {
						store.push(data)
					}
				}

			} else if len(store.data) == len(store.reserved) {
				select {
				case <-done:
					return
				case epoch, ok := <-store.Epoch:
					if !ok {
						return
					}
					store.moveNext(epoch)
				case data, ok := <-store.Push:
					if !ok {
						return
					}
					if len(data) > 0 {
						store.push(data)
					}
				case hash := <-store.hashes:
					if hash.action == mark {
						if _, ok := store.data[hash.hash]; ok {
							store.reserved[hash.hash] = struct{}{}
						}
					} else if hash.action == unmark {
						delete(store.reserved, hash.hash)
					} else if hash.action == exclude {
						delete(store.reserved, hash.hash)
						delete(store.data, hash.hash)
					}
				}
			} else {
				select {
				case <-done:
					return
				case epoch, ok := <-store.Epoch:
					if !ok {
						return
					}
					store.moveNext(epoch)
				case store.Pop <- store.pop():
				case data, ok := <-store.Push:
					if !ok {
						return
					}
					if len(data) > 0 {
						store.push(data)
					}
				case hash := <-store.hashes:
					if hash.action == mark {
						if _, ok := store.data[hash.hash]; ok {
							store.reserved[hash.hash] = struct{}{}
						}
					} else if hash.action == unmark {
						delete(store.reserved, hash.hash)
					} else if hash.action == exclude {
						delete(store.reserved, hash.hash)
						delete(store.data, hash.hash)
					}
				}
			}
		}
	}()
	return store
}

// Mark marks an action as reserved. Marks temporarily exclude the action with
// associated hash from the pool of available actions.
func (a *ActionStore) Mark(hash crypto.Hash) {
	a.hashes <- hashaction{hash, mark}
}

// Unmark unmarks an action as reserved.
func (a *ActionStore) Unmark(hash crypto.Hash) {
	a.hashes <- hashaction{hash, unmark}
}

// Exclude permanently deletes an action from the store.
func (a *ActionStore) Exlude(hash crypto.Hash) {
	a.hashes <- hashaction{hash, exclude}
}

// pop implements the pop operation of the store. The Pop request is sent
// to the Pop channel. pop returns nil if the store is empty. otherwise it
// returns the oldest action in the store (ordered by epoch and order of arrival).
func (a *ActionStore) pop() []byte {
	if len(a.data) == 0 {
		return nil
	}
	for n, hashes := range a.epoch {
		if len(hashes) > 0 {
			cleaned := make([]crypto.Hash, 0, len(hashes))
			for m, hash := range hashes {
				if action, ok := a.data[hash]; ok {
					if _, ok := a.reserved[hash]; !ok {
						a.epoch[n] = append(cleaned, hashes[m:]...)
						delete(a.data, hash)
						return action
					} else {
						// keep reserved but not deleted actions
						cleaned = append(cleaned, hash)
					}
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
	epoch := actions.GetEpochFromByteArray(data)
	if epoch == 0 || epoch+MaxActionDelay < a.currentEpoch || epoch > a.currentEpoch+MaxActionDelay {
		return
	}
	firstBucketEpoch := 0
	if a.currentEpoch > MaxActionDelay {
		firstBucketEpoch = int(a.currentEpoch) - MaxActionDelay
	}
	bucket := int(epoch) - firstBucketEpoch
	hash := crypto.Hasher(data)
	if bucket < 0 || bucket > 2*MaxActionDelay {
		slog.Error("ActionStore: bucket out of range", "bucket", bucket, "epoch", epoch, "current", a.currentEpoch)
		return
	}
	a.epoch[bucket] = append(a.epoch[bucket], hash)
	a.data[hash] = data
}

// moveNext moves the store to the next epoch. It deletes all actions from
// epochs older than MaxActionDelay.
func (a *ActionStore) moveNext(epoch uint64) {
	if epoch != a.currentEpoch+1 {
		slog.Error("ActionStore: non sequential epoch update", "proposed", epoch, "current", a.currentEpoch)
	}
	a.currentEpoch = epoch
	if epoch > MaxActionDelay {
		for _, hash := range a.epoch[0] {
			delete(a.data, hash)
		}
		a.epoch = append(a.epoch[1:], make([]crypto.Hash, 0))
	} else {
		a.epoch = append(a.epoch, make([]crypto.Hash, 0))
	}
}
