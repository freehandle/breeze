package bft

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

type Ballot struct {
	TotalWeight int
	Round       byte
	Proposal    *RoundPropose
	Votes       []*RoundVote
	Commits     []*RoundCommit
}

func PutBallot(b *Ballot, data *[]byte) {
	*data = append(*data, b.Serialize()...)
}

func (b *Ballot) Serialize() []byte {
	bytes := util.Uint64ToBytes(uint64(b.TotalWeight))
	util.PutUint64(uint64(b.TotalWeight), &bytes)
	if b.Proposal != nil {
		util.PutByteArray([]byte{}, &bytes)
	} else {
		util.PutByteArray(b.Proposal.Serialize(), &bytes)
	}
	util.PutUint16(uint16(len(b.Votes)), &bytes)
	for _, vote := range b.Votes {
		util.PutByteArray(vote.Serialize(), &bytes)
	}
	util.PutUint16(uint16(len(b.Votes)), &bytes)
	for _, commit := range b.Commits {
		util.PutByteArray(commit.Serialize(), &bytes)
	}
	return bytes
}

func ParseBallot(data []byte) *Ballot {
	ballot, _ := ParseBallotPosition(data, 0)
	return ballot
}

func ParseBallotPosition(data []byte, position int) (*Ballot, int) {
	var bytes []byte
	ballot := Ballot{
		Votes:   make([]*RoundVote, 0),
		Commits: make([]*RoundCommit, 0),
	}
	var totalweight uint64
	if totalweight, position = util.ParseUint64(data, position); totalweight == 0 {
		return nil, len(data) + 1
	}
	ballot.TotalWeight = int(totalweight)
	ballot.Round, position = util.ParseByte(data, position)
	bytes, position = util.ParseByteArray(data, position)
	if len(bytes) > 0 {
		ballot.Proposal = ParseRoundPropose(bytes)
	}
	var count uint16
	count, position = util.ParseUint16(data, position)
	for i := uint16(0); i < count; i++ {
		bytes, position = util.ParseByteArray(data, position)
		if vote := ParseRoundVote(bytes); vote != nil {
			ballot.Votes = append(ballot.Votes, vote)
		} else {
			return nil, len(data) + 1
		}
	}
	count, position = util.ParseUint16(data, position)
	for i := uint16(0); i < count; i++ {
		bytes, position = util.ParseByteArray(data, position)
		if commit := ParseRoundCommit(bytes); commit != nil {
			ballot.Commits = append(ballot.Commits, commit)
		} else {
			return nil, len(data) + 1
		}
	}
	return &ballot, position
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
