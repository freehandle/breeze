package swell

import "log/slog"

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
