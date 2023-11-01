package store

import (
	"log/slog"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
)

const (
	MaxActionDelay = 100
	mark           = 0
	exclude        = 1
	unmark         = 2
)

type ActionStore struct {
	currentEpoch uint64
	epoch        [][]crypto.Hash
	data         map[crypto.Hash][]byte
	reserved     map[crypto.Hash]struct{}
	Actions      chan []byte
	hashes       chan hashaction
}

type hashaction struct {
	hash   crypto.Hash
	action byte
}

func NewActionStore(epoch uint64) *ActionStore {
	store := &ActionStore{
		data:    make(map[crypto.Hash][]byte),
		Actions: make(chan []byte),
		hashes:  make(chan hashaction),
	}
	if epoch < MaxActionDelay {
		store.epoch = make([][]crypto.Hash, (MaxActionDelay-epoch+1)+MaxActionDelay)
	} else {
		store.epoch = make([][]crypto.Hash, 2*MaxActionDelay+1)
	}
	go func() {
		for {
			select {
			case data, ok := <-store.Actions:
				if !ok {
					return
				}
				if len(data) > 0 {
					store.push(data)
				}
			case store.Actions <- store.pop():
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
	}()
	return store
}

func (a *ActionStore) Mark(hash crypto.Hash) {
	a.hashes <- hashaction{hash, mark}
}

func (a *ActionStore) Unmark(hash crypto.Hash) {
	a.hashes <- hashaction{hash, unmark}
}

func (a *ActionStore) Exlude(hash crypto.Hash) {
	a.hashes <- hashaction{hash, exclude}
}

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

func (a *ActionStore) push(data []byte) {
	epoch := actions.GetEpochFromByteArray(data)
	if epoch == 0 || epoch < a.currentEpoch-MaxActionDelay || epoch > a.currentEpoch+MaxActionDelay {
		return
	}
	bucket := int(epoch - a.currentEpoch - MaxActionDelay)
	hash := crypto.Hasher(data)
	a.epoch[bucket] = append(a.epoch[bucket], hash)
	a.data[hash] = data
}

func (a *ActionStore) moveNext(epoch uint64) {
	if epoch != a.currentEpoch+1 {
		slog.Error("ActionStore: non sequential epoch update", "proposed", epoch, "current", a.currentEpoch)
	}
	a.currentEpoch = epoch

	if epoch-MaxActionDelay-1 > 0 {
		for _, hash := range a.epoch[0] {
			delete(a.data, hash)
		}
		a.epoch = append(a.epoch[1:], make([]crypto.Hash, 0))
	} else {
		a.epoch = append(a.epoch, make([]crypto.Hash, 0))
	}
}
