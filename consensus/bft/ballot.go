package bft

import (
	"github.com/freehandle/breeze/crypto"
)

type Ballot struct {
	TotalWeight int
	Round       byte
	Proposal    *RoundPropose
	Votes       []*RoundVote
	Commits     []*RoundCommit
}

func NewBallot(round byte, weight int) *Ballot {
	return &Ballot{
		TotalWeight: weight,
		Round:       round,
		Votes:       make([]*RoundVote, 0),
		Commits:     make([]*RoundCommit, 0),
	}
}

func (b *Ballot) VoteWeight(value crypto.Hash) int {
	weight := 0
	for _, vote := range b.Votes {
		if (!vote.Blank) && vote.Value.Equal(value) {
			weight += vote.Weight
		}
	}
	return weight
}

func (b *Ballot) Weight() int {
	weight := 0
	for _, vote := range b.Votes {
		weight += vote.Weight
	}
	for _, commit := range b.Commits {
		exists := false
		for _, vote := range b.Votes {
			if commit.Token.Equal(vote.Token) {
				exists = true
				break
			}
		}
		if !exists {
			weight += commit.Weight
		}
	}
	return weight
}

func (b *Ballot) IncoporateVote(vote *RoundVote) (*RoundVote, int) {
	weight := 0
	for _, cast := range b.Votes {
		if (!cast.Blank) && cast.Value.Equal(vote.Value) {
			weight += cast.Weight
		}
		if cast.Token.Equal(vote.Token) {
			if cast.Value.Equal(vote.Value) && (cast.HasHash != vote.HasHash || cast.Blank != vote.Blank) {
				cast.Weight = 0
				vote.Weight = 0
				return cast, 0
			}
		}
	}
	b.Votes = append(b.Votes, vote)
	weight = vote.Weight
	return nil, weight
}

func (b *Ballot) IncoporateCommit(commit *RoundCommit) (*RoundCommit, int) {
	weight := 0
	for _, cast := range b.Commits {
		if (!cast.Blank) && cast.Value.Equal(commit.Value) {
			weight += cast.Weight
		}
		if cast.Token.Equal(commit.Token) {
			if !cast.Value.Equal(commit.Value) {
				cast.Weight = 0
				commit.Weight = 0
				return cast, 0
			}
		}
	}
	b.Commits = append(b.Commits, commit)
	weight += commit.Weight
	return nil, weight
}
func (b *Ballot) HasConsensus() bool {
	if b.Proposal == nil {
		return false
	}
	weight := 0
	for _, vote := range b.Votes {
		if (!vote.Blank) && vote.Value.Equal(b.Proposal.Value) {
			weight += vote.Weight
		}

	}
	return (weight > 2*b.TotalWeight/3)
}

func (b *Ballot) HasMajorityForValue(hash crypto.Hash) bool {
	weight := 0
	for _, vote := range b.Votes {
		if (!vote.Blank) && vote.Value.Equal(b.Proposal.Value) {
			weight += vote.Weight
		}

	}
	return (weight > 2*b.TotalWeight/3)
}

func (b *Ballot) HasBlankConsensus() bool {
	weight := 0
	for _, vote := range b.Votes {
		if vote.Blank {
			weight += vote.Weight
		}

	}
	return (weight > 2*b.TotalWeight/3)
}

func (b *Ballot) HasQuorum() bool {
	weight := 0
	for _, vote := range b.Votes {
		weight += vote.Weight
	}
	return (weight >= 2*F+1)
}

func (b *Ballot) HasCommitQuorum() bool {
	weight := 0
	for _, commit := range b.Commits {
		weight += commit.Weight
	}
	return (weight > 2*b.TotalWeight/3)
}

func (b *Ballot) Finalized() (crypto.Hash, bool) {
	weights := make(map[crypto.Hash]int)
	for _, commit := range b.Commits {
		if !commit.Blank {
			if weight, ok := weights[commit.Value]; ok {
				weight += commit.Weight
				if weight > 2*b.TotalWeight/3 {
					return commit.Value, true
				}
				weights[commit.Value] = weight
			} else {
				if commit.Weight > 2*b.TotalWeight/3 {
					return commit.Value, true
				}
				weights[commit.Value] = commit.Weight
			}
		}
	}
	return crypto.ZeroHash, false
}
