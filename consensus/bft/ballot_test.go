package bft

import (
	"errors"
	"testing"

	"github.com/freehandle/breeze/crypto"
)

func propose(pk crypto.PrivateKey) *RoundPropose {
	propose := &RoundPropose{
		Epoch:     20,
		Round:     1,
		Token:     pk.PublicKey(),
		Value:     crypto.HashToken(pk.PublicKey()),
		LastRound: 0,
	}
	propose.Sign(pk)
	return propose
}

func vote(pk crypto.PrivateKey) *RoundVote {
	vote := &RoundVote{
		Epoch:   20,
		Round:   1,
		Blank:   false,
		Token:   pk.PublicKey(),
		Value:   crypto.HashToken(pk.PublicKey()),
		HasHash: true,
	}
	vote.Sign(pk)
	return vote
}

func commit(pk crypto.PrivateKey) *RoundCommit {
	commit := &RoundCommit{
		Epoch: 20,
		Round: 1,
		Blank: false,
		Token: pk.PublicKey(),
		Value: crypto.HashToken(pk.PublicKey()),
	}
	commit.Sign(pk)
	return commit
}

type serializer interface {
	Serialize() []byte
}

func doublePass(v serializer, p func([]byte) serializer) error {
	msg := v.Serialize()
	v2 := p(msg)
	if v2 == nil {
		return errors.New("Unable to parse")
	}
	msg2 := v2.Serialize()
	if len(msg) != len(msg2) {
		return errors.New("Message length mismatch")
	}
	for n := 0; n < len(msg); n++ {
		if msg[n] != msg2[n] {
			return errors.New("Message mismatch")
		}
	}
	return nil
}

func parseProposse(data []byte) serializer {
	return ParseRoundPropose(data)
}

func parseVote(data []byte) serializer {
	return ParseRoundVote(data)
}

func parseCommit(data []byte) serializer {
	return ParseRoundCommit(data)
}

func parseBallot(data []byte) serializer {
	return ParseBallot(data)
}

func TestBallotSerialization(t *testing.T) {
	_, pk := crypto.RandomAsymetricKey()
	p := propose(pk)
	v := vote(pk)
	c := commit(pk)
	ballot := &Ballot{
		TotalWeight: 100,
		Round:       1,
		Proposal:    p,
		Votes:       []*RoundVote{v},
		Commits:     []*RoundCommit{c},
	}

	if err := doublePass(p, parseProposse); err != nil {
		t.Fatalf("propose: %s", err)
	}

	if err := doublePass(v, parseVote); err != nil {
		t.Fatalf("vote: %s", err)
	}

	if err := doublePass(c, parseCommit); err != nil {
		t.Fatalf("commit: %s", err)
	}

	if err := doublePass(ballot, parseBallot); err != nil {
		t.Fatalf("ballot: %s", err)
	}

}
