package swell

import (
	"log/slog"
	"sync"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

const (
	MaxPoolSize      = 10
	MaxCommitteeSize = 100
)

type Node struct {
	checkpoint  uint64
	blockchain  *chain.Blockchain
	actions     *store.ActionStore
	credentials crypto.PrivateKey
	channel     []*socket.ChannelConnection
	buffered    []*socket.BufferedChannel
	validators  []socket.CommitteeMember
	weights     map[crypto.Token]int
	order       []crypto.Token
}

// Node keeps forming blocks either proposing its own blocks or validating
// others nodes proposals. In due time node re-arranges validator pool.
// Uppon exclusion a node can transition to a listener node.

func (n *Node) RunEpoch(epoch uint64) chan struct{} {
	leaderCount := int(epoch-n.checkpoint) % len(n.order)
	committee := &PoolingCommittee{
		Epoch:   epoch,
		members: make(map[crypto.Token]PoolingMembers),
		order:   make([]crypto.Token, 0),
	}
	channels := make([]*socket.ChannelConnection, 0)
	buffers := make([]*socket.BufferedChannel, 0)
	for i := 0; i < MaxPoolSize; i++ {
		token := n.order[(leaderCount+i)%len(n.order)]
		if member, ok := committee.members[token]; ok {
			committee.members[token] = PoolingMembers{Weight: member.Weight + 1}
		} else {
			committee.members[token] = PoolingMembers{Weight: 1}
			for _, c := range n.channel {
				if c.Is(token) {
					channels = append(n.channel, c)
					break
				}
			}
			for _, b := range n.buffered {
				if b.Is(token) {
					buffers = append(n.buffered, b)
					break
				}
			}
		}
		n.order = append(n.order, token)
	}
	committee.gossip = socket.NewGossip(channels)
	pool := LaunchPooling(*committee, n.credentials)
	leader := n.order[leaderCount]
	if leader.Equal(n.credentials.PublicKey()) {

	}
}

type Proofer interface {
	Punish(duplicates *Duplicate)
	DeterminePool(chain *chain.Chain, candidates []ValidatorCandidate, epoch uint64) Validators
}

type retrievalStatus struct {
	mu     sync.Mutex
	done   bool
	output chan *chain.SealedBlock
}

func (r *retrievalStatus) Done(sealed *chain.SealedBlock) bool {
	mu.Lock()
	defer mu.Unlock()
	if r.done {
		return true
	}
	if sealed != nil {
		r.done = true
		r.output <- sealed
		return true
	}
	return false
}

func RetrieveBlock(epoch uint64, hash crypto.Hash, order []*socket.BufferedChannel) chan *chain.SealedBlock {
	output := make(chan *chain.SealedBlock)
	ellapse := 400 * time.Millisecond
	msg := chain.RequestBlockMessage(epoch, hash)
	status := retrievalStatus{
		mu:     sync.Mutex{},
		output: output,
	}
	for n, channel := range order {
		go func(n int, channel *socket.BufferedChannel, status *retrievalStatus) {
			time.Sleep(time.Duration(n) * ellapse)
			if status.Done() {
				return
			}
			channel.SendSide(msg)
			data := channel.ReadSide()
			if len(data) == 0 {
				return
			}
			sealed := chain.ParseSealedBlock(data)
			if sealed != nil && sealed.Header.Epoch == epoch && sealed.Seal.Hash.Equal(hash) {
				status.Done(sealed)
				return
			}
		}(n, channel, &status)
	}
	return output
}

func ListenToBlock(leader *socket.BufferedChannel, others []*socket.BufferedChannel, pool *Pooling, blockchain *chain.Blockchain) bool {
	var sealed *chain.SealedBlock
	go func() {
		var block *chain.BlockBuilder
		for {
			data := leader.Read()
			if len(data) == 0 {
				continue
			}
			switch data[0] {
			case chain.MsgNewBlock:
				header := chain.ParseBlockHeader(data[1:])
				if header == nil {
					slog.Info("ListenToBlock: invalid block header")
					return
				}
				block = blockchain.CheckpointValidator(*header)
				if block == nil {
					slog.Info("ListenToBlock: invalid block header")
					pool.SealBlock(crypto.ZeroHash)
					return
				}
			case chain.MsgSealBlock:
				epoch, position := util.ParseUint64(data, 1)
				if block == nil || epoch != block.Header.Epoch {
					slog.Info("ListenToBlock: invalid epoch on seal")
					return
				}
				seal := chain.ParseBlockSeal(data[position:])
				if seal == nil {
					slog.Info("ListenToBlock: invalid seal")
					return
				}
				sealed = block.ImprintSeal(*seal)
				pool.SealBlock(seal.Hash)
				return
			case chain.MsgAction:
				if block != nil {
					if !block.Validate(data[1:]) {
						slog.Info("ListenToBlock: invalid action")
					}
				}
			}
		}
	}()
	consensus := <-pool.Finalize
	if !consensus.Value.Equal(sealed.Seal.Hash) {
		nodesWithData := make(map[crypto.Token]struct{})
		for _, round := range consensus.Rounds {
			for _, vote := range round.Votes {
				if vote.HasHash && vote.Value.Equal(consensus.Value) {
					nodesWithData[vote.Token] = struct{}{}
				}
			}
		}
		order := make([]*socket.BufferedChannel, 0)
		for token, _ := range nodesWithData {
			for _, others := range others {
				if others.Is(token) {
					order = append(order, others)
					break
				}
			}
		}
		sealed = <-RetrieveBlock(sealed.Header.Epoch, consensus.Value, order)
	}
	if sealed == nil {
		return false
	}
	blockchain.AddSealedBlock(sealed)
	return true
}

func BuildBlock(epoch uint64, blockchain *chain.Blockchain, broadcast *socket.BroadcastPool, actions *store.ActionStore, credentials crypto.PrivateKey, pool *Pooling) bool {
	timeout := time.NewTimer(980 * time.Millisecond)
	header := blockchain.NextBlock(epoch)
	if header == nil {
		return false
	}
	block := blockchain.CheckpointValidator(*header)
	msg := chain.NewBlockMessage(*header)
	broadcast.Send(msg)
	var sealed *chain.SealedBlock
	go func() {
		for {
			select {
			case action := <-actions.Actions:
				if len(action) > 0 && block.Validate(action) {
					msg := chain.ActionMessage(action)
					broadcast.Send(msg)
				}
			case <-timeout.C:
				sealed = block.Seal(credentials)
				hash := crypto.ZeroHash
				if sealed != nil {
					hash = sealed.Seal.Hash
					msg := chain.BlockSealMessage(epoch, sealed.Seal)
					broadcast.Send(msg)
				} else {
					slog.Warn("BuildBlock: could not seal own block")
				}
				pool.SealBlock(hash)
				return
			}
		}
	}()
	consensus := <-pool.Finalize
	if sealed != nil && consensus.Value.Equal(sealed.Seal.Hash) {
		blockchain.AddSealedBlock(sealed)
		return true
	} else if consensus.Value.Equal(crypto.ZeroHash) {

		return false
	}
	return true
}
