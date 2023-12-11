package chain

import "github.com/freehandle/breeze/crypto"

// IncorporatedActions is a data structure that keeps track of the actions
// incorporated in the blockchain. It is used to prevent the same action to be
// incorporated twice.
type IncorporatedActions struct {
	CurrentEpoch     uint64
	incorporated     map[uint64]map[crypto.Hash]uint64
	maxProtocolEpoch uint64
}

func NewIncorporatedActions(epoch, MaxProtocolEpoch uint64) *IncorporatedActions {
	return &IncorporatedActions{
		CurrentEpoch:     epoch,
		incorporated:     make(map[uint64]map[crypto.Hash]uint64),
		maxProtocolEpoch: MaxProtocolEpoch,
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
	delete(ia.incorporated, ia.CurrentEpoch-ia.maxProtocolEpoch)
	ia.CurrentEpoch += 1
}
