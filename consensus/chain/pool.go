package chain

import "github.com/freehandle/breeze/crypto"

const MaxProtocolEpoch = 100

type IncorporatedActions struct {
	CurrentEpoch uint64
	incorporated map[uint64]map[crypto.Hash]uint64
}

func NewIncorporatedActions(epoch uint64) *IncorporatedActions {
	return &IncorporatedActions{
		CurrentEpoch: epoch,
		incorporated: make(map[uint64]map[crypto.Hash]uint64),
	}
}

func (ia *IncorporatedActions) Append(hash crypto.Hash, epoch uint64) {
	if epochHashes, ok := ia.incorporated[epoch]; ok {
		epochHashes[hash] = epoch
	} else {
		ia.incorporated[epoch] = map[crypto.Hash]uint64{hash: epoch}
	}
}

func (ia *IncorporatedActions) IsNew(hash crypto.Hash, epoch uint64, checkpoint uint64) bool {
	if epochHashes, ok := ia.incorporated[epoch]; ok {
		incorporation, exists := epochHashes[hash]
		return !exists && (incorporation <= checkpoint)
	}
	return true
}

func (ia *IncorporatedActions) MoveForward() {
	delete(ia.incorporated, ia.CurrentEpoch-MaxProtocolEpoch)
	ia.CurrentEpoch += 1
}
