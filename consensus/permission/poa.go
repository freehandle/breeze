package permission

import (
	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
)

// NewProofOfStake returns a new empty ProofOfStake.
func NewProofOfAuthority(tokens ...crypto.Token) *ProofOfAuthority {
	return &ProofOfAuthority{
		Authorized: tokens,
	}
}

// Proof of authority implements a PoA permission interface.
type ProofOfAuthority struct {
	Authorized []crypto.Token
}

func (poa *ProofOfAuthority) Authorize(token crypto.Token) {
	for _, t := range poa.Authorized {
		if t.Equal(token) {
			return
		}
	}
	poa.Authorized = append(poa.Authorized, token)
}

// Cancel removes a token from the list of authorized tokens.
func (poa *ProofOfAuthority) Cancel(token crypto.Token) {
	for i, t := range poa.Authorized {
		if t.Equal(token) {
			poa.Authorized = append(poa.Authorized[:i], poa.Authorized[i+1:]...)
			return
		}
	}
}

// Punish returns an empty map of punishments and cancel the autorization of the
// violators to participate in consensus.
func (poa *ProofOfAuthority) Punish(duplicates *bft.Duplicate, weights map[crypto.Token]int) map[crypto.Token]uint64 {
	violations := Violations(duplicates)
	punishments := make(map[crypto.Token]uint64)
	for token := range violations {
		poa.Cancel(token)
	}
	return punishments
}

// DeterminePool returns a map of authorized tokens and their equal weight of 1.
func (poa *ProofOfAuthority) DeterminePool(chain *chain.Blockchain, candidates []crypto.Token) map[crypto.Token]int {
	validated := make(map[crypto.Token]int)
	for _, candidate := range candidates {
		for _, token := range poa.Authorized {
			if candidate.Equal(token) {
				validated[token] = 1
			}
		}
	}
	return validated
}
