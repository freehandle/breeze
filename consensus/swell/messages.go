package swell

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

const (
	RoundProposeMsg = 0
	RoundVoteMsg    = 1
	RoundCommitMsg  = 2
	DuplicateMsg    = 3
	DoneMsg         = 4
	CandidateMsg    = 5
)

type ConsensusMessage interface {
	Serialize() []byte
	MsgKind() byte
}

type Done struct {
	Epoch     uint64
	Token     crypto.Token
	Signature crypto.Signature
}

func (d *Done) Serialize() []byte {
	bytes := []byte{4}
	util.PutUint64(d.Epoch, &bytes)
	util.PutToken(d.Token, &bytes)
	util.PutSignature(d.Signature, &bytes)
	return bytes
}

func (d *Done) MsgKind() byte {
	return 4
}

func ParseDone(msg []byte) *Done {
	if len(msg) != 1+crypto.TokenSize+crypto.SignatureSize+8 || msg[0] != 4 {
		return nil
	}
	done := &Done{}
	position := 1
	done.Epoch, position = util.ParseUint64(msg, position)
	done.Token, position = util.ParseToken(msg, position)
	done.Signature, _ = util.ParseSignature(msg, position)
	if !done.Token.Verify(msg[:position], done.Signature) {
		return nil
	}
	return done
}

func NewDone(epoch uint64, credentials crypto.PrivateKey) *Done {
	done := &Done{Epoch: epoch, Token: credentials.PublicKey()}
	bytes := []byte{4}
	util.PutUint64(done.Epoch, &bytes)
	util.PutToken(done.Token, &bytes)
	done.Signature = credentials.Sign(bytes)
	return done
}

// RoundPropose is used by the leader of the consensys pool for a givven epoch and
// at a given round to propose a value for the hash of the block for the epoch.
// By swell rules a honest node will only propose a value it has received from
// the leader at round 0 or a zero value hash.
// A honest node will send the LastRound for which it has recevied 2F+1 votes
// for the anounced value. If it has bot received, LastRound = 0
type RoundPropose struct {
	Epoch     uint64
	Round     byte
	Token     crypto.Token
	Value     crypto.Hash
	LastRound byte
	Signatute crypto.Signature
}

func ParseRoundPropose(bytes []byte) *RoundPropose {
	if (len(bytes) != 11+crypto.TokenSize+crypto.Size+crypto.SignatureSize) || bytes[0] != RoundProposeMsg {
		return nil
	}
	position := 1
	vote := RoundPropose{}
	vote.Epoch, position = util.ParseUint64(bytes, position)
	vote.Token, position = util.ParseToken(bytes, position)
	vote.Value, position = util.ParseHash(bytes, position)
	vote.LastRound, position = util.ParseByte(bytes, position)
	if position > len(bytes) {
		return nil
	}
	vote.Signatute, _ = util.ParseSignature(bytes, position)
	if vote.Token.Verify(bytes[:position], vote.Signatute) {
		return &vote
	} else {
		return nil
	}
}

func (r *RoundPropose) MsgKind() byte {
	return 0
}

func (r *RoundPropose) serializeToSign() []byte {
	bytes := []byte{0}
	util.PutUint64(r.Epoch, &bytes)
	util.PutByte(r.Round, &bytes)
	util.PutToken(r.Token, &bytes)
	util.PutHash(r.Value, &bytes)
	util.PutByte(r.LastRound, &bytes)
	return bytes
}

func (r *RoundPropose) Serialize() []byte {
	bytes := r.serializeToSign()
	util.PutSignature(r.Signatute, &bytes)
	return bytes
}

func (r *RoundPropose) Sign(key crypto.PrivateKey) {
	r.Signatute = key.Sign(r.serializeToSign())
}

// Nodes of the pool will vote for a value in each round or casy a blank vote.
// If the vote is blank the information about the Value and HasHas is just to
// inform others that it is in possession of data for the given hash.
// Each vote is given weight proportional to the node stake.
type RoundVote struct {
	Epoch     uint64
	Round     byte
	Blank     bool
	Token     crypto.Token
	Value     crypto.Hash
	HasHash   bool
	Signatute crypto.Signature
	Weight    int
}

func ParseRoundVote(bytes []byte) *RoundVote {
	if len(bytes) != 12+crypto.TokenSize+crypto.Size+crypto.SignatureSize || bytes[0] != RoundVoteMsg {
		return nil
	}
	position := 1
	vote := RoundVote{}
	vote.Epoch, position = util.ParseUint64(bytes, position)
	vote.Blank, position = util.ParseBool(bytes, position)
	vote.Token, position = util.ParseToken(bytes, position)
	vote.Value, position = util.ParseHash(bytes, position)
	vote.HasHash, position = util.ParseBool(bytes, position)
	if position > len(bytes) {
		return nil
	}
	vote.Signatute, _ = util.ParseSignature(bytes, position)
	if vote.Token.Verify(bytes[:position], vote.Signatute) {
		return &vote
	} else {
		return nil
	}
}

func (r *RoundVote) MsgKind() byte {
	return 1
}

func (r *RoundVote) serializeToSign() []byte {
	bytes := []byte{1}
	util.PutUint64(r.Epoch, &bytes)
	util.PutByte(r.Round, &bytes)
	util.PutBool(r.Blank, &bytes)
	util.PutToken(r.Token, &bytes)
	util.PutHash(r.Value, &bytes)
	util.PutBool(r.HasHash, &bytes)
	return bytes
}

func (r *RoundVote) Serialize() []byte {
	bytes := r.serializeToSign()
	util.PutSignature(r.Signatute, &bytes)
	return bytes
}

func (r *RoundVote) Sign(key crypto.PrivateKey) {
	r.Signatute = key.Sign(r.serializeToSign())
}

type RoundCommit struct {
	Epoch     uint64
	Round     byte
	Blank     bool
	Token     crypto.Token
	Value     crypto.Hash
	Signatute crypto.Signature
	Weight    int
}

func ParseRoundCommit(bytes []byte) *RoundCommit {
	if len(bytes) != 11+crypto.TokenSize+crypto.Size+crypto.SignatureSize || bytes[0] != RoundCommitMsg {
		return nil
	}
	position := 1
	vote := RoundCommit{}
	vote.Epoch, position = util.ParseUint64(bytes, position)
	vote.Round, position = util.ParseByte(bytes, position)
	vote.Blank, position = util.ParseBool(bytes, position)
	vote.Token, position = util.ParseToken(bytes, position)
	vote.Value, position = util.ParseHash(bytes, position)
	if position > len(bytes) {
		return nil
	}
	vote.Signatute, _ = util.ParseSignature(bytes, position)
	if vote.Token.Verify(bytes[:position], vote.Signatute) {
		return &vote
	} else {
		return nil
	}
}

func (r *RoundCommit) MsgKind() byte {
	return 2
}

func (r *RoundCommit) serializeToSign() []byte {
	bytes := []byte{2}
	util.PutUint64(r.Epoch, &bytes)
	util.PutByte(r.Round, &bytes)
	util.PutBool(r.Blank, &bytes)
	util.PutToken(r.Token, &bytes)
	util.PutHash(r.Value, &bytes)
	return bytes
}

func (r *RoundCommit) Serialize() []byte {
	bytes := r.serializeToSign()
	util.PutSignature(r.Signatute, &bytes)
	return bytes
}

func (r *RoundCommit) Sign(key crypto.PrivateKey) {
	r.Signatute = key.Sign(r.serializeToSign())
}

type Ballot struct {
	Round    byte
	Proposal *RoundPropose
	Votes    []*RoundVote
	Commits  []*RoundCommit
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

func NewBallot(round byte) *Ballot {
	return &Ballot{
		Round:   round,
		Votes:   make([]*RoundVote, 0),
		Commits: make([]*RoundCommit, 0),
	}
}

func (b *Ballot) IncoporateVote(vote *RoundVote) (*RoundVote, int) {
	weight := 0
	b.Votes = append(b.Votes, vote)
	for _, cast := range b.Votes {
		if (!cast.Blank) && cast.Value.Equal(vote.Value) {
			weight += cast.Weight
		}
		if cast.Token.Equal(vote.Token) {
			if cast.Value.Equal(vote.Value) || cast.HasHash != vote.HasHash || cast.Blank != vote.Blank {
				cast.Weight = 0
				vote.Weight = 0
				return cast, 0
			}
		}
	}
	return nil, weight
}

func (b *Ballot) IncoporateCommit(commit *RoundCommit) (*RoundCommit, int) {
	weight := 0
	b.Commits = append(b.Commits, commit)
	for _, cast := range b.Commits {
		if (!cast.Blank) && cast.Value.Equal(commit.Value) {
			weight += cast.Weight
		}
		if cast.Token.Equal(commit.Token) {
			if cast.Value.Equal(commit.Value) {
				cast.Weight = 0
				commit.Weight = 0
				return cast, 0
			}
		}
	}
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
	return (weight >= 2*F+1)
}

func (b *Ballot) HasMajorityForValue(hash crypto.Hash) bool {
	weight := 0
	for _, vote := range b.Votes {
		if (!vote.Blank) && vote.Value.Equal(b.Proposal.Value) {
			weight += vote.Weight
		}

	}
	return (weight >= 2*F+1)
}

func (b *Ballot) HasBlankConsensus() bool {
	weight := 0
	for _, vote := range b.Votes {
		if vote.Blank {
			weight += vote.Weight
		}

	}
	return (weight >= 2*F+1)
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
	return (weight >= 2*F+1)
}

func (b *Ballot) Finalized() (crypto.Hash, bool) {
	weights := make(map[crypto.Hash]int)
	for _, commit := range b.Commits {
		if !commit.Blank {
			if weight, ok := weights[commit.Value]; ok {
				weight += commit.Weight
				if weight >= 2*F+1 {
					return commit.Value, true
				}
				weights[commit.Value] = weight
			} else {
				if commit.Weight >= 2*F+1 {
					return commit.Value, true
				}
				weights[commit.Value] = commit.Weight
			}
		}
	}
	return crypto.ZeroHash, false
}
