package bft

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type GossipNetwork interface {
	Broadcast([]byte)
	BroadcastExcept([]byte, crypto.Token)
	ReleaseToken(crypto.Token)
	Messages() chan socket.GossipMessage
}

func LaunchPooling(committee PoolingCommittee, credentials crypto.PrivateKey) *Pooling {
	pooling := &Pooling{
		committee:   committee,
		credentials: credentials,
		Finalize:    make(chan *ConsensusCommit),
		rounds:      make([]*Ballot, 0),
		duplicates:  NewDuplicate(),
		shutdown:    make(chan struct{}),
	}
	messages := committee.Gossip.Messages()
	go func() {
		pooling.NewRound(0)
		for {
			select {
			case <-pooling.shutdown:
				return
			case msg := <-messages:
				if len(msg.Signal) == 0 {
					continue
				}
				switch msg.Signal[0] {
				case RoundProposeMsg:
					propose := ParseRoundPropose(msg.Signal)
					if propose == nil {
						continue
					}
					committee.Gossip.BroadcastExcept(msg.Signal, msg.Token)
					if pooling.isLeader(msg.Token, propose.Round) {
						round := pooling.getRound(propose.Round)
						if round.Proposal == nil {
							round.Proposal = propose
							pooling.Check()
						} else {
							pooling.duplicates.AddProposal(round.Proposal, propose)
						}
					}
				case RoundVoteMsg:
					vote := ParseRoundVote(msg.Signal)
					if vote == nil {
						continue
					}
					if w, ok := committee.Members[vote.Token]; ok {
						vote.Weight = w.Weight
					}
					committee.Gossip.BroadcastExcept(msg.Signal, msg.Token)
					round := pooling.getRound(vote.Round)
					another, _ := round.IncoporateVote(vote)
					if another != nil {
						pooling.duplicates.AddVote(another, vote)
					} else {
						pooling.Check()
					}
				case RoundCommitMsg:
					commit := ParseRoundCommit(msg.Signal)
					if commit == nil {
						continue
					}
					if w, ok := committee.Members[commit.Token]; ok {
						commit.Weight = w.Weight
					}
					committee.Gossip.BroadcastExcept(msg.Signal, msg.Token)
					round := pooling.getRound(commit.Round)
					another, _ := round.IncoporateCommit(commit)
					if another != nil {
						pooling.duplicates.AddCommit(another, commit)
					} else {
						pooling.Check()
					}
				case DoneMsg:
					committee.Gossip.ReleaseToken(msg.Token)
				}
			}
		}
	}()
	return pooling
}
