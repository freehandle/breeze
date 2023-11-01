package swell

import (
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

const (
	MaxPoolSize      = 10
	MaxCommitteeSize = 100
)

type Node struct {
	checkpoint  uint64
	blockchain  *chain.Chain
	credentials crypto.PrivateKey
	Gossip      *socket.Gossip
	Broadcast   *socket.BroadcastPool
	validators  []socket.CommitteeMember
	weights     map[crypto.Token]int
}

func (n *Node) NewBlock(epoch uint64) {

}

type Proofer interface {
	Punish(duplicates *Duplicate)
	DeterminePool(chain *chain.Chain, candidates []ValidatorCandidate, epoch uint64) Validators
}

func LaucnhGenesis(proofer Proofer) {

}

func BuildBlock(committee *socket.BroadcastPool, actions *store.ActionStore, blockchain *chain.Chain) chan crypto.Hash {
	sealHash := make(chan crypto.Hash)
	timeout := time.NewTimer(980 * time.Millisecond)
	epoch := blockchain.LiveBlock.Header.Epoch
	blockchain.NextBlock(epoch + 1)
	msg := chain.NewBlockMessage(blockchain.LiveBlock.Header)
	committee.Send(msg)
	go func() {
		for {
			select {
			case action := <-actions.Actions:
				if len(action) > 0 && blockchain.Validate(action) {
					msg := chain.ActionMessage(action)
					committee.Send(msg)
				}
			case <-timeout.C:
				seal := blockchain.SealOwnBlock()
				msg := chain.BlockSealMessage(blockchain.LiveBlock.Header.Epoch, seal)
				committee.Send(msg)
				sealHash <- seal.Hash
				return
			}
		}
	}()
	return sealHash
}
