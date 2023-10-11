package state

import (
	"github.com/freehandle/breeze/crypto"
)

type Mutations struct {
	Epoch         uint64
	DeltaWallets  map[crypto.Hash]int
	DeltaDeposits map[crypto.Hash]int
}

func NewMutations(epoch uint64) *Mutations {
	return &Mutations{
		Epoch:         epoch,
		DeltaWallets:  make(map[crypto.Hash]int),
		DeltaDeposits: make(map[crypto.Hash]int),
	}
}

func (m *Mutations) GetEpoch() uint64 {
	return m.Epoch
}

func (m *Mutations) DeltaBalance(hash crypto.Hash) int {
	value := m.DeltaWallets[hash]
	return value
}

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
