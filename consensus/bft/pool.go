package bft

import (
	"time"

	"github.com/freehandle/breeze/crypto"
)

const (
	F = 2
	//ConsensusMajority = 2*f + 1
	//SimpleMajority    = f + 1
)

const (
	Proposing ConsensusState = iota
	Voting
	Committing
)

var (
	TimeOutCommit  = 1000 * time.Millisecond
	TimeOutVote    = 1000 * time.Millisecond
	TimeOutPropose = 1000 * time.Millisecond
)

type ConsensusState byte

type ConsensusCommit struct {
	Value      crypto.Hash
	Rounds     []*Ballot
	Duplicates *Duplicate
}

type PoolingMembers struct {
	Weight int
}

type PoolingCommittee struct {
	Epoch   uint64
	Members map[crypto.Token]PoolingMembers
	Gossip  GossipNetwork
	Order   []crypto.Token
}

func (p PoolingCommittee) TotalWeight() int {
	total := 0
	for _, member := range p.Members {
		total += member.Weight
	}
	return total
}

type Pooling struct {
	round          byte
	state          ConsensusState
	validHash      *crypto.Hash
	validHashRound byte
	commitHash     *crypto.Hash
	commitRound    byte
	blockSeal      crypto.Hash
	pendingVote    *RoundVote // vote waiting timout and hash confirmation
	rounds         []*Ballot
	timerOutVote   bool
	committee      PoolingCommittee
	credentials    crypto.PrivateKey
	duplicates     *Duplicate
	Finalize       chan *ConsensusCommit
	shutdown       chan struct{}
}

func (p *Pooling) isLeader(token crypto.Token, round byte) bool {
	return p.committee.Order[int(round)%len(p.committee.Order)].Equal(token)
}

func (p *Pooling) getRound(r byte) *Ballot {
	if len(p.rounds) > int(r) {
		return p.rounds[r]
	}
	for round := len(p.rounds); round <= int(r); round++ {
		p.rounds = append(p.rounds, NewBallot(byte(round), p.committee.TotalWeight()))
	}
	return p.rounds[r]
}

func (p *Pooling) SealBlock(hash crypto.Hash) {
	p.blockSeal = hash
	if p.committee.Order[0].Equal(p.credentials.PublicKey()) {
		if p.committee.Order[0].Equal(p.credentials.PublicKey()) {
			if p.round == 0 && p.state == Proposing {
				p.CastPropose()
				p.state = Voting
			}
		}
	}
}

func (p *Pooling) NewRound(r byte) {
	p.round = r
	p.state = Proposing
	if p.round > byte(len(p.rounds)) {
		for r := len(p.rounds); r <= int(r); r++ {
			p.rounds = append(p.rounds, NewBallot(byte(r), p.committee.TotalWeight()))
		}
	}
	leader := p.committee.Order[int(p.round)%len(p.committee.Order)]
	if leader.Equal(p.credentials.PublicKey()) && !p.blockSeal.Equal(crypto.ZeroHash) {
		p.CastPropose()
		p.state = Voting
	} else {
		p.SetTimeoutPropose(p.round)
	}
	p.pendingVote = nil
	p.timerOutVote = false
}

func (p *Pooling) TimeoutPropose(r byte) {
	if p.round == r && p.state == Proposing {
		if p.pendingVote != nil {
			p.pendingVote.Sign(p.credentials)
			p.CastVote(p.pendingVote.Value, p.has(p.pendingVote.Value))
			p.pendingVote = nil
		} else {
			p.CastBlankVote()
		}
		p.state = Voting
	}
}

func (p *Pooling) TimeoutVote(r byte) {
	if p.round == r && p.state == Voting {
		p.CastBlankCommit()
		p.state = Committing
	}
}

func (p *Pooling) TimeoutCommit(r byte) {
	if p.round == r && p.state == Committing {
		p.NewRound(r + 1)
	}
}

func (p *Pooling) Check() {
	round := p.rounds[p.round]
	proposal := round.Proposal

	// in any state

	// terminate with 2F+1 commit to value
	if hash, ok := round.Finalized(); ok {
		done := NewDone(p.committee.Epoch, p.credentials)
		p.Broadcast(done.Serialize())
		p.Finalize <- &ConsensusCommit{Value: hash, Rounds: p.rounds, Duplicates: p.duplicates}
		p.shutdown <- struct{}{}
		return
	}

	// timeout after 2F+1 commuit to any value
	if round.HasCommitQuorum() {
		p.SetTimeoutCommit(p.round)
	}

	// move to posterior round with 2F+1 messages of any kind
	if len(p.rounds) > int(p.round) {
		for n := int(p.round); n < len(p.rounds); n++ {
			if p.rounds[n].Weight() > 2*F {
				p.NewRound(byte(n))
			}
		}
	}

	// while proposing or voting

	if p.state == Proposing || p.state == Voting {
		if proposal != nil && round.HasMajorityForValue(proposal.Value) {
			hash := proposal.Value
			p.validHash = &hash
			p.validHashRound = p.round
		}
	}

	// while proposing

	if p.state == Proposing && proposal != nil {
		if proposal.LastRound == 0 {
			if hash := p.commitHash; hash == nil || (*hash).Equal(proposal.Value) {
				if p.has(proposal.Value) {
					p.CastVote(proposal.Value, true)
					p.state = Voting
				} else {
					p.PendVote(proposal.Value)
				}
			}
		} else {
			if p.rounds[proposal.LastRound-1].HasMajorityForValue(proposal.Value) {
				if !p.isContraryToCommit(proposal.Value, proposal.LastRound) {
					p.CastVote(proposal.Value, p.has(proposal.Value))
					p.state = Voting
				} else {
					p.CastBlankVote()
					p.state = Voting
				}
			}
		}
	}

	// while state is voting do:

	if p.state == Voting {

		if round.HasQuorum() {
			if !p.timerOutVote {
				p.SetTimeoutVote(p.round)
			}
		}

		if proposal := round.Proposal; proposal != nil && round.HasMajorityForValue(proposal.Value) {
			p.CastCommit(proposal.Value)
			p.state = Committing
		}

		if round.HasBlankConsensus() {
			p.CastBlankCommit()
			p.state = Committing
		}
	}

}

func (p *Pooling) SetTimeoutPropose(r byte) {
	go func() {
		time.Sleep(TimeOutPropose)
		p.TimeoutPropose(r)
	}()
}

func (p *Pooling) SetTimeoutVote(r byte) {
	go func() {
		time.Sleep(TimeOutVote)
		p.TimeoutVote(r)
	}()
}

func (p *Pooling) SetTimeoutCommit(r byte) {
	go func() {
		time.Sleep(TimeOutCommit)
		p.TimeoutCommit(r)
	}()
}

func (p *Pooling) isContraryToCommit(value crypto.Hash, round byte) bool {
	if p.commitHash == nil {
		return false
	}
	if p.commitHash.Equal(value) {
		return false
	}
	if p.commitRound < round {
		return false
	}
	return true
}

func (p *Pooling) has(hash crypto.Hash) bool {
	return true
}

func (p *Pooling) CastVote(hash crypto.Hash, has bool) {
	token := p.credentials.PublicKey()
	vote := &RoundVote{
		Epoch:   p.committee.Epoch,
		Round:   p.round,
		Token:   token,
		Value:   hash,
		HasHash: has,
		Weight:  p.weight(token),
	}
	vote.Sign(p.credentials)
	p.Broadcast(vote.Serialize())

}

func (p *Pooling) PendVote(hash crypto.Hash) {
	token := p.credentials.PublicKey()
	vote := &RoundVote{
		Epoch:  p.committee.Epoch,
		Round:  p.round,
		Token:  token,
		Value:  hash,
		Weight: p.weight(token),
	}
	p.pendingVote = vote
}

func (p *Pooling) CastBlankVote() {
	token := p.credentials.PublicKey()
	vote := &RoundVote{
		Epoch:  p.committee.Epoch,
		Round:  p.round,
		Blank:  true,
		Token:  token,
		Weight: p.weight(token),
	}
	vote.Sign(p.credentials)
	p.Broadcast(vote.Serialize())
}

func (p *Pooling) CastCommit(hash crypto.Hash) {
	token := p.credentials.PublicKey()
	commit := &RoundCommit{
		Epoch:  p.committee.Epoch,
		Round:  p.round,
		Token:  token,
		Value:  hash,
		Weight: p.weight(token),
	}
	commit.Sign(p.credentials)
	p.Broadcast(commit.Serialize())
}

func (p *Pooling) CastBlankCommit() {
	token := p.credentials.PublicKey()
	commit := &RoundCommit{
		Epoch:  p.committee.Epoch,
		Round:  p.round,
		Blank:  true,
		Token:  token,
		Weight: p.weight(token),
	}
	commit.Sign(p.credentials)
	p.Broadcast(commit.Serialize())
}

func (p *Pooling) CastPropose() {
	token := p.credentials.PublicKey()
	propose := &RoundPropose{
		Epoch: p.committee.Epoch,
		Round: p.round,
		Token: token,
	}
	if p.validHash == nil {
		if p.round == 0 {
			propose.Value = p.blockSeal
		} else {
			propose.Value = crypto.ZeroHash
		}
	} else {
		propose.Value = *p.validHash
		propose.LastRound = p.validHashRound
	}
	propose.Sign(p.credentials)
	p.Broadcast(propose.Serialize())

}

func (p *Pooling) weight(token crypto.Token) int {
	member := p.committee.Members[token]
	return member.Weight
}

func (p *Pooling) Broadcast(msg []byte) {
	p.committee.Gossip.Broadcast(msg)
}
