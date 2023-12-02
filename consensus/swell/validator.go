package swell

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

type SwellNetworkConfiguration struct {
	NetworkHash      crypto.Hash
	MaxPoolSize      int
	MaxCommitteeSize int
	BlockInterval    time.Duration
	ChecksumWindow   int
	Permission       Permission
}

const (
	MaxPoolSize      = 10
	MaxCommitteeSize = 100
)

type Permission interface {
	Punish(duplicates *bft.Duplicate, weights map[crypto.Token]int) map[crypto.Token]uint64
	DeterminePool(chain *chain.Blockchain, candidates []crypto.Token) Validators
}

type BlockConsensusConfirmation struct {
	Epoch  uint64
	Status bool
}

type ValidatingNode struct {
	//genesisTime  time.Time
	window      uint64            // epoch starting current checksum window
	blockchain  *chain.Blockchain // nodes of distinct windows can have this pointer concurrently
	actions     *store.ActionStore
	credentials crypto.PrivateKey
	committee   *ChecksumWindowValidatorPool
	swellConfig SwellNetworkConfiguration
	relay       *relay.Node
}

func (c *ValidatingNode) IsPoolMember(epoch uint64) bool {
	token := c.credentials.PublicKey()
	leader := int(epoch-c.window) % len(c.committee.order)
	for n := 0; n < c.swellConfig.MaxCommitteeSize; n++ {
		if token.Equal(c.committee.order[(leader+n)%len(c.committee.order)]) {
			return true
		}
	}
	return false
}

func RunValidatorNode(ctx context.Context, node *ValidatingNode, window int) {
	windowStartEpoch := uint64(window*node.swellConfig.ChecksumWindow + 1)
	epoch := windowStartEpoch
	waiting := node.blockchain.Timer(epoch)

	mintConfirmation := make(chan BlockConsensusConfirmation)

	go func() {
		done := ctx.Done()
		for {
			select {
			case <-waiting.C:
				if node.IsPoolMember(epoch) {
					node.RunEpoch(epoch, mintConfirmation)
				}
				epoch += 1
				waiting = node.blockchain.Timer(epoch)
			case <-done:
				waiting.Stop()
				return
			}
		}
	}()

	go func() {
		sealedblocks := make(map[uint64]crypto.Hash, 0)
		commitblocks := make(map[uint64]crypto.Hash, 0)
		cancel := ctx.Done()
		for {
			select {
			case syncRequest := <-node.relay.SyncRequest:
				if syncRequest.State {
					go node.blockchain.SyncState(syncRequest.Conn)
				} else {
					go node.blockchain.SyncBlocksServer(syncRequest.Conn, syncRequest.Epoch)
				}
			case consensus := <-mintConfirmation:
				if consensus.Status {
					for _, sealed := range node.blockchain.SealedBlocks {
						if hash, ok := sealedblocks[sealed.Header.Epoch]; !ok {
							sealedblocks[sealed.Header.Epoch] = sealed.Seal.Hash
							msg := chain.SealedBlockMessage(sealed)
							node.relay.BlockEvents <- msg
						} else if !hash.Equal(sealed.Seal.Hash) {
							// todo: can this happen?
						}
					}
					for _, commit := range node.blockchain.RecentBlocks {
						if commit.Header.Epoch >= windowStartEpoch {
							if hash, ok := commitblocks[commit.Header.Epoch]; !ok {
								commitblocks[commit.Header.Epoch] = commit.Seal.Hash
								msg := chain.CommitBlockMessage(commit.Header.Epoch, commit.Commit)
								node.relay.BlockEvents <- msg
							} else if !hash.Equal(commit.Seal.Hash) {
								// todo: can this happen?
							}
						}
					}
				} else {
					slog.Warn("validator consensus failed for epoch", "epoch", consensus.Epoch)
				}
			case <-cancel:
				close(mintConfirmation)
				return
			}
		}
	}()
}

// Node keeps forming blocks either proposing its own blocks or validating
// others nodes proposals. In due time node re-arranges validator pool.
// Uppon exclusion a node can transition to a listener node.
func (n *ValidatingNode) RunEpoch(epoch uint64, done chan BlockConsensusConfirmation) {
	leaderCount := int(epoch-n.window) % len(n.committee.order)
	leaderToken := n.committee.order[leaderCount]
	committee := &bft.PoolingCommittee{
		Epoch:   epoch,
		Members: make(map[crypto.Token]bft.PoolingMembers),
		Order:   make([]crypto.Token, 0),
	}
	peers := make([]socket.CommitteeMember, 0)
	for i := 0; i < MaxPoolSize; i++ {
		token := n.committee.order[(leaderCount+i)%len(n.committee.order)]
		weight := n.committee.weights[token]
		if weight == 0 {
			slog.Warn("RunEpoch: zero weight member")
			continue
		}
		if member, ok := committee.Members[token]; ok {
			committee.Members[token] = bft.PoolingMembers{Weight: member.Weight + weight}
		} else {
			committee.Members[token] = bft.PoolingMembers{Weight: weight}
		}
		for _, v := range n.committee.validators {
			if v.Token.Equal(token) {
				peers = append(peers, v)
				break
			}
		}
	}
	bftConnections := socket.AssembleChannelNetwork(peers, n.credentials, 5401, n.committee.consensus)
	committee.Gossip = socket.GroupGossip(epoch, bftConnections)
	pool := bft.LaunchPooling(*committee, n.credentials)
	leader := n.committee.order[leaderCount]
	go func() {
		ok := false
		if leader.Equal(n.credentials.PublicKey()) {
			ok = BuildBlock(epoch, n.blockchain, n.committee.blocks, n.actions, n.credentials, pool)
		} else {
			leader, others := n.committee.blocks.GetLeader(leaderToken)
			if leader != nil {
				ok = ListenToBlock(leader, others, pool, n.blockchain)
			}
		}
		done <- BlockConsensusConfirmation{Epoch: epoch, Status: ok}
	}()
}

func (n *ValidatingNode) ListenEpoch(epoch uint64, done chan BlockConsensusConfirmation) {

}

type retrievalStatus struct {
	mu     sync.Mutex
	done   bool
	output chan *chain.SealedBlock
}

func (r *retrievalStatus) Done(sealed *chain.SealedBlock) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
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

func (v *ValidatingNode) ListeToBroadcastChannel(epoch uint64) {

}

func ListenToBlock(leader *socket.BufferedChannel, others []*socket.BufferedChannel, pool *bft.Pooling, blockchain *chain.Blockchain) bool {
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
					slog.Info("ListenToBlock: invalid ep1och on seal")
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
		for token := range nodesWithData {
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

func BuildBlock(epoch uint64, blockchain *chain.Blockchain, broadcast *socket.PercolationPool, actions *store.ActionStore, credentials crypto.PrivateKey, pool *bft.Pooling) bool {
	timeout := time.NewTimer(980 * time.Millisecond)
	header := blockchain.NextBlock(epoch)
	if header == nil {
		return false
	}
	block := blockchain.CheckpointValidator(*header)
	msg := chain.NewBlockMessage(*header)
	broadcast.Send(epoch, msg)
	var sealed *chain.SealedBlock
	go func() {
		for {
			select {
			case action := <-actions.Actions:
				if len(action) > 0 && block.Validate(action) {
					msg := chain.ActionMessage(action)
					broadcast.Send(epoch, msg)
				}
			case <-timeout.C:
				sealed = block.Seal(credentials)
				hash := crypto.ZeroHash
				if sealed != nil {
					hash = sealed.Seal.Hash
					msg := chain.BlockSealMessage(epoch, sealed.Seal)
					broadcast.Send(epoch, msg)
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
