package state

import (
	"github.com/freehandle/breeze/crypto"
)

// Mutation is a change in the state of a wallet or a deposit kept in memory by
// a golang hashmap from the hash of token into deltas of wallets and deposits.
type Mutations struct {
	Epoch         uint64
	DeltaWallets  map[crypto.Hash]int
	DeltaDeposits map[crypto.Hash]int
}

// NewMutations creates a new mutation object with the given epoch.
func NewMutations(epoch uint64) *Mutations {
	return &Mutations{
		Epoch:         epoch,
		DeltaWallets:  make(map[crypto.Hash]int),
		DeltaDeposits: make(map[crypto.Hash]int),
	}
}

// GetEpoch returns the epoch of the mutation.
func (m *Mutations) GetEpoch() uint64 {
	return m.Epoch
}

// DeltaBalance retuns the delta of the balance of the given hash.
func (m *Mutations) DeltaBalance(hash crypto.Hash) int {
	value := m.DeltaWallets[hash]
	return value
}

// Append mutations into a single mutation object with epoch given by the
// caller of the method.
func (m *Mutations) Append(array []*Mutations) *Mutations {
	grouped := NewMutations(m.Epoch)
	all := []*Mutations{m}
	if len(array) > 0 {
		for _, mutation := range array {
			if mutation.Epoch >= grouped.Epoch {
				grouped.Epoch = mutation.Epoch
			}
			all = append(all, mutation)

		}
	}
	for _, mutations := range all {
		for hash, delta := range mutations.DeltaWallets {
			if value, ok := grouped.DeltaWallets[hash]; ok {
				grouped.DeltaWallets[hash] = value + delta
			} else {
				grouped.DeltaWallets[hash] = delta
			}
		}
		for hash, delta := range mutations.DeltaDeposits {
			if value, ok := grouped.DeltaDeposits[hash]; ok {
				grouped.DeltaDeposits[hash] = value + delta
			} else {
				grouped.DeltaDeposits[hash] = delta
			}
		}
	}
	return grouped
}
