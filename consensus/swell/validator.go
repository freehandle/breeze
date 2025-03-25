package swell

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
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
	startEpoch := c.Node.blockchain.Timer(epoch)
	slog.Debug("RunValidator: starting new window", "starting at", epoch, "ending at", c.End, "validators", c.Committee.validators)
	// to receive confirmations from the goroutines responsi
	c.newBlock = make(chan BlockConsensusConfirmation)
	//checksumEpoch := (c.Start + c.End) / 2
	//hasCheckpoint := make(chan bool)
	//requestedChecksum := false
	terminated := false

	go func() {
		done := c.ctx.Done()
		defer func() {
			close(c.newBlock)
		}()
		for {
			select {
			case <-startEpoch.C:
				fmt.Println(time.Now(), "Epoch", epoch)
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
							go c.BuildSoloBLock(epoch)
						} else {
							go c.RunEpoch(epoch)
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
