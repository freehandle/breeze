package bft

import (
	"log/slog"

	"github.com/freehandle/breeze/util"
)

type DuplicateVote struct {
	One *RoundVote
	Two *RoundVote
}

type DuplicateCommit struct {
	One *RoundCommit
	Two *RoundCommit
}

type DuplicateProposal struct {
	One *RoundPropose
	Two *RoundPropose
}

type Duplicate struct {
	Votes     []DuplicateVote
	Commits   []DuplicateCommit
	Proposals []DuplicateProposal
}

func NewDuplicate() *Duplicate {
	return &Duplicate{
		Votes:     make([]DuplicateVote, 0),
		Commits:   make([]DuplicateCommit, 0),
		Proposals: make([]DuplicateProposal, 0),
	}
}

func PutDuplicate(d *Duplicate, bytes *[]byte) {
	util.PutUint16(uint16(len(d.Votes)), bytes)
	for _, vote := range d.Votes {
		util.PutByteArray(vote.One.Serialize(), bytes)
		util.PutByteArray(vote.Two.Serialize(), bytes)
	}
	util.PutUint16(uint16(len(d.Commits)), bytes)
	for _, commit := range d.Commits {
		util.PutByteArray(commit.One.Serialize(), bytes)
		util.PutByteArray(commit.Two.Serialize(), bytes)
	}
	util.PutUint16(uint16(len(d.Proposals)), bytes)
	for _, proposal := range d.Proposals {
		util.PutByteArray(proposal.One.Serialize(), bytes)
		util.PutByteArray(proposal.Two.Serialize(), bytes)
	}
}

func (d *Duplicate) Serialize() []byte {
	bytes := make([]byte, 0)
	PutDuplicate(d, &bytes)
	return bytes
}

func ParseDuplicate(data []byte) *Duplicate {
	parsed, _ := ParseDuplicatePosition(data, 0)
	return parsed
}

func ParseDuplicatePosition(data []byte, position int) (*Duplicate, int) {
	duplicate := NewDuplicate()
	var count uint16
	count, position = util.ParseUint16(data, position)
	var one, two []byte
	for i := uint16(0); i < count; i++ {
		one, position = util.ParseByteArray(data, position)
		two, position = util.ParseByteArray(data, position)
		oneVote := ParseRoundVote(one)
		twoVote := ParseRoundVote(two)
		if oneVote == nil || twoVote == nil {
			return nil, len(data) + 1
		}
		duplicate.AddVote(oneVote, twoVote)
	}
	count, position = util.ParseUint16(data, position)
	for i := uint16(0); i < count; i++ {
		one, position = util.ParseByteArray(data, position)
		two, position = util.ParseByteArray(data, position)
		oneCommit := ParseRoundCommit(one)
		twoCommit := ParseRoundCommit(two)
		if oneCommit == nil || twoCommit == nil {
			return nil, len(data) + 1
		}
		duplicate.AddCommit(oneCommit, twoCommit)
	}
	count, position = util.ParseUint16(data, position)
	for i := uint16(0); i < count; i++ {
		one, position = util.ParseByteArray(data, position)
		two, position = util.ParseByteArray(data, position)
		onePropose := ParseRoundPropose(one)
		twoPropose := ParseRoundPropose(two)
		if onePropose == nil || twoPropose == nil {
			return nil, len(data)
		}

		duplicate.AddProposal(onePropose, twoPropose)
	}
	return duplicate, position
}

func (d *Duplicate) HasViolations() bool {
	return len(d.Commits) > 0 || len(d.Proposals) > 0 || len(d.Votes) > 0
}

func (d *Duplicate) AddVote(one, two *RoundVote) {
	slog.Info("duplicate vote", "token", one.Token, "epoch", one.Epoch, "round", one.Round)
	d.Votes = append(d.Votes, DuplicateVote{one, two})
}

func (d *Duplicate) AddCommit(one, two *RoundCommit) {
	slog.Info("duplicate commit", "token", one.Token, "epoch", one.Epoch, "round", one.Round)
	d.Commits = append(d.Commits, DuplicateCommit{one, two})
}

func (d *Duplicate) AddProposal(one, two *RoundPropose) {
	slog.Info("duplicate proposal", "token", one.Token, "epoch", one.Epoch, "round", one.Round)
	d.Proposals = append(d.Proposals, DuplicateProposal{one, two})
}

/*func DenounceDuplicate(one, another ConsensusMessage) []byte {
	bytes := []byte{3, one.MsgKind()}
	bytes = append(bytes, one.Serialize()...)
	bytes = append(bytes, another.Serialize()...)
	return bytes
}

func AttestDenounceDuplicate(msg []byte) bool {
	if len(msg) < 2 || msg[0] != 3 || msg[1] > 2 {
		return false
	}
	position := 1
	if msg[1] == 0 {
		var one, another *RoundPropose
		one, position = ParseRoundPropose(msg, position)
		another, position = ParseRoundPropose(msg, position)
		if one == nil || another == nil || position != len(msg) {
			return false
		}
		if one.Epoch != another.Epoch {
			return false
		}
		if one.Round != another.Round {
			return false
		}
		if !one.Token.Equal(another.Token) {
			return false
		}
		return one.LastRound != another.LastRound || (!one.Value.Equal(another.Value))
	}
	if msg[1] == 1 {
		var one, another *RoundVote
		one, position = ParseRoundVote(msg, position)
		another, position = ParseRoundVote(msg, position)
		if one == nil || another == nil || position != len(msg) {
			return false
		}
		if one.Epoch != another.Epoch {
			return false
		}
		if one.Round != another.Round {
			return false
		}
		if !one.Token.Equal(another.Token) {
			return false
		}
		return one.HasHash != another.HasHash || (!one.Value.Equal(another.Value)) || one.Blank != another.Blank
	}
	if msg[1] == 2 {
		var one, another *RoundCommit
		one, position = ParseRoundCommit(msg, position)
		another, position = ParseRoundCommit(msg, position)
		if one == nil || another == nil || position != len(msg) {
			return false
		}
		if one.Epoch != another.Epoch {
			return false
		}
		if one.Round != another.Round {
			return false
		}
		if !one.Token.Equal(another.Token) {
			return false
		}
		return !one.Value.Equal(another.Value) || one.Blank != another.Blank
	}
	return false
}
*/
