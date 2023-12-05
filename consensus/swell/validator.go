package swell

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

func (node *SwellNode) RunValidatingNode(ctx context.Context, committee *ChecksumWindowValidatorPool, window int) {

	windowStartEpoch := uint64(window*node.config.ChecksumWindow + 1)
	epoch := windowStartEpoch
	waiting := node.blockchain.Timer(epoch)
	mintConfirmation := make(chan BlockConsensusConfirmation)

	go func() {
		done := ctx.Done()
		for {
			select {
			case <-waiting.C:
				if node.IsPoolMember(epoch) {
					node.RunEpoch(epoch, committee, mintConfirmation)
				}
				epoch += 1
				waiting = node.blockchain.Timer(epoch)
				fmt.Println("epoch", epoch)
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

func (c *SwellNode) IsPoolMember(epoch uint64) bool {
	token := c.credentials.PublicKey()
	windowStart := (int(epoch)/c.config.ChecksumWindow)*c.config.ChecksumWindow + 1
	leader := int(epoch-uint64(windowStart)) % len(c.validators.order)
	for n := 0; n < c.config.MaxCommitteeSize; n++ {
		if token.Equal(c.validators.order[(leader+n)%len(c.validators.order)]) {
			return true
		}
	}
	return false
}

// Node keeps forming blocks either proposing its own blocks or validating
// others nodes proposals. In due time node re-arranges validator pool.
// Uppon exclusion a node can transition to a listener node.
func (node *SwellNode) RunEpoch(epoch uint64, network *ChecksumWindowValidatorPool, done chan BlockConsensusConfirmation) {
	windowStart := (int(epoch)/node.config.ChecksumWindow)*node.config.ChecksumWindow + 1
	leaderCount := (int(epoch) - windowStart) % len(node.validators.order)
	leaderToken := node.validators.order[leaderCount]
	committee := &bft.PoolingCommittee{
		Epoch:   epoch,
		Members: make(map[crypto.Token]bft.PoolingMembers),
		Order:   make([]crypto.Token, 0),
	}
	peers := make([]socket.CommitteeMember, 0)
	for i := 0; i < MaxPoolSize; i++ {
		token := node.validators.order[(leaderCount+i)%len(node.validators.order)]
		weight := node.validators.weights[token]
		if weight == 0 {
			slog.Warn("RunEpoch: zero weight member")
			continue
		}
		if member, ok := committee.Members[token]; ok {
			committee.Members[token] = bft.PoolingMembers{Weight: member.Weight + weight}
		} else {
			committee.Members[token] = bft.PoolingMembers{Weight: weight}
		}
		for _, v := range network.validators {
			if v.Token.Equal(token) {
				peers = append(peers, v)
				break
			}
		}
	}
	bftConnections := socket.AssembleChannelNetwork(peers, node.credentials, 5401, node.hostname, network.consensus)
	committee.Gossip = socket.GroupGossip(epoch, bftConnections)
	pool := bft.LaunchPooling(*committee, node.credentials)
	leader := node.validators.order[leaderCount]
	go func() {
		ok := false
		if leader.Equal(node.credentials.PublicKey()) {
			ok = node.BuildBlock(epoch, network, pool)
		} else {
			leader, others := network.blocks.GetLeader(leaderToken)
			if leader != nil {
				ok = node.ListenToBlock(leader, others, pool)
			}
		}
		done <- BlockConsensusConfirmation{Epoch: epoch, Status: ok}
	}()
}

func (node *SwellNode) BuildBlock(epoch uint64, network *ChecksumWindowValidatorPool, pool *bft.Pooling) bool {
	timeout := time.NewTimer(980 * time.Millisecond)
	header := node.blockchain.NextBlock(epoch)
	if header == nil {
		return false
	}
	block := node.blockchain.CheckpointValidator(*header)
	msg := chain.NewBlockMessage(*header)
	network.blocks.Send(epoch, msg)
	var sealed *chain.SealedBlock
	go func() {
		for {
			select {
			case action := <-node.actions.Actions:
				if len(action) > 0 && block.Validate(action) {
					msg := chain.ActionMessage(action)
					network.blocks.Send(epoch, msg)
				}
			case <-timeout.C:
				sealed = block.Seal(node.credentials)
				hash := crypto.ZeroHash
				if sealed != nil {
					hash = sealed.Seal.Hash
					msg := chain.BlockSealMessage(epoch, sealed.Seal)
					network.blocks.Send(epoch, msg)
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
		node.blockchain.AddSealedBlock(sealed)
		return true
	} else if consensus.Value.Equal(crypto.ZeroHash) {
		return false
	}
	return true
}

func (node *SwellNode) ListenToBlock(leader *socket.BufferedChannel, others []*socket.BufferedChannel, pool *bft.Pooling) bool {
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

				block = node.blockchain.CheckpointValidator(*header)
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
	node.blockchain.AddSealedBlock(sealed)
	return true
}
