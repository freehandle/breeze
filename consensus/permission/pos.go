package pos

import (
	"log/slog"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
)

type ProofOfStake struct {
	MinimumStage uint64
}

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

func (pos *ProofOfStake) Punish(duplicates *bft.Duplicate, weights map[crypto.Token]int) map[crypto.Token]uint64 {
	violations := Violations(duplicates)
	punishments := make(map[crypto.Token]uint64)
	for token := range violations {
		punishments[token] = uint64(weights[token]) * pos.MinimumStage
	}
	return punishments
}

func (pos *ProofOfStake) DeterminePool(chain *chain.Blockchain, candidates []crypto.Token) swell.Validators {
	validated := make(swell.Validators, 0)
	for _, token := range candidates {
		_, deposit := chain.Checksum.State.Deposits.Balance(token)
		if deposit >= pos.MinimumStage {
			validated = append(validated, &swell.Validator{
				Token:  token,
				Weight: int(deposit / pos.MinimumStage),
			})
		}
	}
	return validated
}

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

func (poa *ProofOfAuthority) Cancel(token crypto.Token) {
	for i, t := range poa.Authorized {
		if t.Equal(token) {
			poa.Authorized = append(poa.Authorized[:i], poa.Authorized[i+1:]...)
			return
		}
	}
}

func (poa *ProofOfAuthority) Punish(duplicates *bft.Duplicate, weights map[crypto.Token]int) map[crypto.Token]uint64 {
	violations := Violations(duplicates)
	punishments := make(map[crypto.Token]uint64)
	for token := range violations {
		poa.Cancel(token)
	}
	return punishments
}

func (poa *ProofOfAuthority) DeterminePool(chain *chain.Blockchain, candidates []crypto.Token) swell.Validators {
	validated := make(swell.Validators, 0)
	for _, candidate := range candidates {
		for _, token := range poa.Authorized {
			if candidate.Equal(token) {
				validated = append(validated, &swell.Validator{
					Token:  token,
					Weight: 1,
				})
			}
		}
	}
	return validated
}
