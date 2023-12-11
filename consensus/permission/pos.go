/*
Permissions implement a permission interface to determine if a node is
allowed to participate in consensus and punish nodes that violate the
consensus rules.

The ProofOfStake permission implementation requires a minimum amount of tokens
staged on a deposit contract to be allowed to participate in consensus. The
balance deposited is checked against the state of the chain at the time of
checksum window creation. The ProofOfStake implementation punishes nodes that
violate the consensus rules by slashing their deposit.

The ProofOfAuthority permission implementation requires a list of authorized
tokens to be allowed to participate in consensus. The ProofOfAuthority does not
contemplate punishment other than be automatically removed from the list of
authorized tokens.
*/
package permission

import (
	"log/slog"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
)

// Proof of statke implements a PoS permission interface.
type ProofOfStake struct {
	MinimumStage uint64
}

// Violations returns a map of tokens that have violated the consensus rules
// according to duplicates evidence.
func Violations(duplicates *bft.Duplicate) map[crypto.Token]struct{} {
	violations := make(map[crypto.Token]struct{})
	for _, duplicate := range duplicates.Votes {
		violations[duplicate.One.Token] = struct{}{}
		slog.Info("Duplicate vote", "token", duplicate.One.Token, "epoch", duplicate.One.Epoch, "round", duplicate.One.Round, "hash", duplicate.One.Value, "duplicate", duplicate.Two.Value)
	}
	for _, duplicate := range duplicates.Commits {
		violations[duplicate.One.Token] = struct{}{}
		slog.Info("Duplicate commit", "token", duplicate.One.Token, "epoch", duplicate.One.Epoch, "round", duplicate.One.Round, "hash", duplicate.One.Value, "duplicate", duplicate.Two.Value)
	}
	for _, duplicate := range duplicates.Proposals {
		violations[duplicate.One.Token] = struct{}{}
		slog.Info("Duplicate proposal", "token", duplicate.One.Token, "epoch", duplicate.One.Epoch, "round", duplicate.One.Round, "hash", duplicate.One.Value, "duplicate", duplicate.Two.Value)
	}
	return violations
}

// Punish returns a map of punishments for the violators with their respective
// stakes for stashing.
func (pos *ProofOfStake) Punish(duplicates *bft.Duplicate, weights map[crypto.Token]int) map[crypto.Token]uint64 {
	violations := Violations(duplicates)
	punishments := make(map[crypto.Token]uint64)
	for token := range violations {
		punishments[token] = uint64(weights[token]) * pos.MinimumStage
	}
	return punishments
}

// DeterminePool returns the autorized candidates with their respective deposits.
func (pos *ProofOfStake) DeterminePool(chain *chain.Blockchain, candidates []crypto.Token) map[crypto.Token]int {
	validated := make(map[crypto.Token]int)
	for _, token := range candidates {
		_, deposit := chain.Checksum.State.Deposits.Balance(token)
		if deposit >= pos.MinimumStage {
			validated[token] = int(deposit / pos.MinimumStage)
		}
	}
	return validated
}

// NewProofOfStake returns a new empty ProofOfStake.
func NewProofOfAuthority() *ProofOfAuthority {
	return &ProofOfAuthority{
		Authorized: make([]crypto.Token, 0),
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
