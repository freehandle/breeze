package swell

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

// Node must be a member of Committee. It starts the execution of the block
// formation for the window.
// RunValidatingNode engine runs the underlying swell node as a validating node.
// This means that it is receiving blocks throught the gossip network of
// validators and at those assigned epochs joins the consensus committes over
// the hash of the block. Honest validators should have a relay network somewhat
// open so that others can send actions to the node and listen to newly minted
// blocks.
func RunValidator(c *Window) {
	epoch := c.Start
	startEpoch := c.Node.Timer(epoch)
	slog.Debug("RunValidator: starting new window", "starting at", epoch, "ending at", c.End, "validators", c.Committee.validators)
	// to receive confirmations from the goroutines responsi
	c.newBlock = make(chan BlockConsensusConfirmation)
	//checksumEpoch := (c.Start + c.End) / 2
	//hasCheckpoint := make(chan bool)
	//requestedChecksum := false
	terminated := false

	go func() {
		done := c.ctx.Done()
		for {
			select {
			case <-startEpoch.C:
				if epoch > c.End {
					if terminated {
						slog.Info("Swell: checksum window ended successfully", "starting at", c.Start, "ending at", c.End)
						return
					}
					// fake block consensus confirmation to wait until all blocks are
					// commit to terminate the windows jow
					c.newBlock <- BlockConsensusConfirmation{Epoch: epoch, Status: true}

				} else {
					//<-epoch
					c.Node.actions.NextEpoch()
					if c.IsPoolMember(epoch) {
						if len(c.Committee.weights) == 1 {
							c.BuildSoloBLock(epoch)
						} else {
							c.RunEpoch(epoch)
						}
					}
				}
				epoch += 1
				startEpoch = c.Node.blockchain.Timer(epoch)
			case <-done:
				slog.Debug("RunValidator: context done, ending timer", "ending at", epoch)
				startEpoch.Stop()
				return
			}
		}
	}()

	go func() {
		defer func() {
			close(c.newBlock)
		}()
		sealedblocks := make(map[uint64]crypto.Hash, 0)
		commitblocks := make(map[uint64]crypto.Hash, 0)
		cancel := c.ctx.Done()
		for {
			select {
			case response := <-c.Node.relay.TopologyRequest:
				if response == nil {
					slog.Error("TopologyRequest with empty connection received")
					continue
				}
				topology := &messages.NetworkTopology{
					Start:      c.Start,
					End:        c.End,
					StartAt:    c.Node.TimeStampBlock(c.Start),
					Order:      c.Committee.order,
					Validators: c.Committee.validators,
				}
				if err := response.Send(topology.Serialize()); err != nil {
					response.Shutdown()
				}
			case syncRequest := <-c.Node.relay.SyncRequest:
				msg := append([]byte{messages.MsgCommittee}, c.Committee.Serialize()...)
				syncRequest.Conn.SendDirect(msg)
				if syncRequest.State {
					go c.Node.blockchain.SyncState(syncRequest.Conn)
				} else {
					go c.Node.blockchain.SyncBlocksServer(syncRequest.Conn, syncRequest.Epoch)
				}
			case statement := <-c.Node.relay.Statement:
				checksumStatement := chain.ParseChecksumStatement(statement)
				if checksumStatement != nil {
					c.unpublished = append(c.unpublished, checksumStatement)
					//c.incorporateStatement(checksumStatement, epoch)
				}
			case consensus := <-c.newBlock:
				if consensus.Status {
					for _, sealed := range c.Node.blockchain.SealedBlocks {
						if hash, ok := sealedblocks[sealed.Header.Epoch]; !ok {
							sealedblocks[sealed.Header.Epoch] = sealed.Seal.Hash
							msg := messages.SealedBlock(sealed.Serialize())
							c.Node.relay.BlockEvents <- msg
						} else if !hash.Equal(sealed.Seal.Hash) {
							slog.Error("SwellNode.RunValidatingNode: ambigous sealed block", "expected hash", crypto.EncodeHash(hash), "got hash", crypto.EncodeHash(sealed.Seal.Hash))
							// todo: can this happen?
						}
					}
					for _, commit := range c.Node.blockchain.RecentBlocks {
						if commit.Header.Epoch >= c.Start {
							if hash, ok := commitblocks[commit.Header.Epoch]; !ok {
								if sealHash, ok := sealedblocks[commit.Header.Epoch]; !ok {
									// if sealed block not already sent, send it
									if sealed := commit.Sealed(); sealed != nil {
										msg := messages.SealedBlock(sealed.Serialize())
										sealedblocks[commit.Header.Epoch] = sealed.Seal.Hash
										c.Node.relay.BlockEvents <- msg
									} else {
										slog.Error("SwellNode.RunValidatingNode: could not get sealed from commit block", "epoch", commit.Header.Epoch)
									}
								} else if !sealHash.Equal(commit.Seal.Hash) {
									slog.Error("SwellNode.RunValidatingNode: multiple sealed blocks for the same epoch", "epoch", commit.Header.Epoch, "existing hash", crypto.EncodeHash(sealHash), "got hash", crypto.EncodeHash(commit.Seal.Hash))
								}
								msg := messages.Commit(commit.Header.Epoch, commit.Seal.Hash, commit.Commit.Serialize())
								commitblocks[commit.Header.Epoch] = commit.Seal.Hash
								c.Node.relay.BlockEvents <- msg
								// terminate job if we reached the end of the window
								// this pressuposes time ordering of recent blocks in
								// blockchain struct.
								if commit.Header.Epoch == c.End {
									terminated = true
									return
								}
							} else if !hash.Equal(commit.Seal.Hash) {
								slog.Error("SwellNode.RunValidatingNode: ambigous commit block", "expected hash", crypto.EncodeHash(hash), "got hash", crypto.EncodeHash(commit.Seal.Hash))
							}
						}
					}
					if c.CanPrepareNextWindow() && c.nextCommittee != nil {
						msg := append([]byte{messages.MsgNextCommittee}, c.nextCommittee.Serialize()...)
						c.Node.relay.BlockEvents <- msg
					}
				} else {
					slog.Warn("validator consensus failed for epoch", "epoch", consensus.Epoch)
				}
			case <-cancel:
				slog.Debug("RunValidator: context done, ending block dealer", "ending at", epoch)
				return
			}
		}
	}()
}

// Node keeps forming blocks either proposing its own blocks or validating
// others nodes proposals. In due time node re-arranges validator pool.
// Uppon exclusion a node can transition to a listener node.
func (w *Window) RunEpoch(epoch uint64) {
	leaderCount := int(epoch-w.Start) % len(w.Committee.order)
	leaderToken := w.Committee.order[leaderCount]
	poolingCommittee := &bft.PoolingCommittee{
		Epoch:   epoch,
		Members: make(map[crypto.Token]bft.PoolingMembers),
		Order:   make([]crypto.Token, 0),
	}
	peers := make([]socket.TokenAddr, 0)
	for i := 0; i < w.Node.config.MaxPoolSize; i++ {
		token := w.Committee.order[(leaderCount+i)%len(w.Committee.order)]
		weight := w.Committee.weights[token]
		if weight == 0 {
			slog.Warn("RunEpoch: zero weight member")
			continue
		}
		poolingCommittee.Order = append(poolingCommittee.Order, token)
		if member, ok := poolingCommittee.Members[token]; ok {
			poolingCommittee.Members[token] = bft.PoolingMembers{Weight: member.Weight + weight}
		} else {
			poolingCommittee.Members[token] = bft.PoolingMembers{Weight: weight}
			for _, v := range w.Committee.validators {
				if v.Token.Equal(token) {
					peers = append(peers, v)
					break
				}
			}
		}
	}
	bftConnections := socket.AssembleChannelNetwork(w.ctx, peers, w.Node.credentials, 5401, w.Node.hostname, w.Committee.consensus)
	poolingCommittee.Gossip = socket.GroupGossip(epoch, bftConnections)

	pool := bft.LaunchPooling(*poolingCommittee, w.Node.credentials)
	go func() {
		ok := false
		if leaderToken.Equal(w.Node.credentials.PublicKey()) {
			ok = w.BuildBlock(epoch, pool)
		} else {
			leader, others := w.Committee.blocks.GetLeader(leaderToken)
			if leader != nil {
				ok = w.ListenToBlock(leader, others, pool)
			}
		}
		w.newBlock <- BlockConsensusConfirmation{Epoch: epoch, Status: ok}
	}()
}

// BuildSoloBLock builds a block in the case where the node is the sole partipant
// in the validating network. In this case all the extra burden can be eliminated
// and the node can build, seal, commit and broadcast a block in a single step.
func (w *Window) BuildSoloBLock(epoch uint64) bool {
	timeout := time.NewTimer(980 * time.Millisecond)
	block := w.StartNewBlock(epoch)
	if block == nil {
		return false
	}
	for {
		select {
		case action := <-w.Node.actions.Pop:
			if len(action.Data) > 0 && block.Validate(action.Data) {
				// clear actionarray
			}
		case <-timeout.C:
			sealed := block.Seal(w.Node.credentials)
			if sealed == nil {
				slog.Warn("BuildBlock: could not seal own block")
				return false
			} else {
				w.AddSealedBlock(sealed)
				w.Node.relay.BlockEvents <- messages.SealedBlock(sealed.Serialize())
				return true
			}
		}
	}
}

// BuildBlock build a new block according to the available state of the swell
// node at the calling of this method. The block is broadcasted to the gossip
// network and the pool consensus committee is launched. Once terminated the
// node cast a proposal for the given hash on the pool network.
func (w *Window) BuildBlock(epoch uint64, pool *bft.Pooling) bool {
	timeout := time.NewTimer(980 * time.Millisecond)
	block := w.StartNewBlock(epoch)
	if block == nil {
		return false
	}
	msg := messages.NewBlockMessage(block.Header.Serialize())
	w.Committee.blocks.Send(epoch, msg)
	var sealed *chain.SealedBlock
	go func() {
		for {
			select {
			case action := <-w.Node.actions.Pop:
				fmt.Println("****", action)
				if len(action.Data) > 0 && block.Validate(action.Data) {
					msg := messages.ActionMessage(action.Data)
					w.Committee.blocks.Send(epoch, msg)
				}
			case <-timeout.C:
				sealed = block.Seal(w.Node.credentials)
				hash := crypto.ZeroHash
				if sealed != nil {
					hash = sealed.Seal.Hash
					msg := messages.BlockSealMessage(epoch, sealed.Seal.Serialize())
					w.Committee.blocks.Send(epoch, msg)
				} else {
					slog.Warn("BuildBlock: could not seal own block")
				}
				pool.SealBlock(hash, w.Node.credentials.PublicKey())
				return
			}
		}
	}()
	consensus := <-pool.Finalize
	if sealed != nil && consensus.Value.Equal(sealed.Seal.Hash) {
		sealed.Seal.Consensus = consensus.Rounds
		w.AddSealedBlock(sealed)
		return true
	} else if consensus.Value.Equal(crypto.ZeroHash) {
		return false
	}
	return false
}

// ListenToBlock listens to the block events from the gossip network and upon
// receiving a swal informs the pool consensus committee about the hash of the
// proposed block. If the pool returns a valid consensus the block is added as
// a sealed block to the node. In case the swell node is not in posession of a
// block with the consensus hash it tries to get that block from other nodes
// of the gossip network.
func (w *Window) ListenToBlock(leader *socket.BufferedMultiChannel, others []*socket.BufferedMultiChannel, pool *bft.Pooling) bool {
	defer leader.Release(pool.Epoch())
	var sealed *chain.SealedBlock
	epoch := pool.Epoch()
	go func() {
		var block *chain.BlockBuilder
		for {
			data := leader.Read(epoch)
			if len(data) == 0 {
				continue
			}
			switch data[0] {
			case messages.MsgNewBlock:
				header := chain.ParseBlockHeader(data[1:])
				if header == nil {
					slog.Info("ListenToBlock: invalid block header")
					return
				}
				block = w.Node.blockchain.CheckpointValidator(*header)
				if block == nil {
					slog.Info("ListenToBlock: invalid block header")
					pool.SealBlock(crypto.ZeroHash, crypto.ZeroToken)
					return
				}
			case messages.MsgSeal:
				epoch, position := util.ParseUint64(data, 1)
				seal := chain.ParseBlockSeal(data[position:])
				if seal == nil {
					slog.Info("ListenToBlock: invalid seal", "epoch", epoch)
					return
				}
				if block == nil {
					slog.Info("ListenToBlock: received seal without block header", "epoch", epoch, "seal", crypto.EncodeHash(seal.Hash))
					return
				}
				if epoch != block.Header.Epoch {
					slog.Info("ListenToBlock: received seal incompatible with block header epoch")
					return
				}
				sealed = block.ImprintSeal(*seal)
				pool.SealBlock(seal.Hash, block.Header.Proposer)
				return
			case messages.MsgAction:
				if block != nil {
					if !block.Validate(data[1:]) {
						slog.Info("ListenToBlock: invalid action")
					}
				}
			}
		}
	}()
	consensus := <-pool.Finalize

	if consensus == nil {
		slog.Error("ListenToBlock: nil consensus received from channel")
		return false
	}

	if sealed == nil || (!consensus.Value.Equal(sealed.Seal.Hash)) {
		nodesWithData := make(map[crypto.Token]struct{})
		for _, round := range consensus.Rounds {
			for _, vote := range round.Votes {
				if vote.HasHash && vote.Value.Equal(consensus.Value) {
					nodesWithData[vote.Token] = struct{}{}
				}
			}
		}
		order := make([]*socket.BufferedMultiChannel, 0)
		for token := range nodesWithData {
			for _, others := range others {
				if others.Is(token) {
					order = append(order, others)
					break
				}
			}
		}
		sealed = <-RetrieveBlock(pool.Epoch(), consensus.Value, order)
	}
	if sealed == nil {
		slog.Warn("Breeze: ListentToBlock could not retrieve sealed block compatible consensus")
		return false
	}
	sealed.Seal.Consensus = consensus.Rounds
	w.AddSealedBlock(sealed)
	return true
}
