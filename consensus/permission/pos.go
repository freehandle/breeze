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
