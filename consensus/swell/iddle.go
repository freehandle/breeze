package swell

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/socket"
)

// IddleNode keep listening for new blocks from a relay connecition to a node
// on the current validating pool. If block servng node is droped from the next 
// checksum or the connection is lost, window IddleNode will try to connect to
// another node on the pool. If node cannot connect to any other node, it will
// return and close the activation channel. 
func RunIddleNode(ctx context.Context, node *ValidatingNode, conn *socket.SignedConnection) chan bool {
	activate := make(chan bool)

	newSealed := make(chan *chain.SealedBlock)
	go func() {
		for {
			msg, err := conn.Read()
			if err != nil {
				return
			}
			if len(msg) < 1 {
				continue
			}
			switch msg[0] {
			case chain.MsgSyncError:
				conn.Shutdown()
				close(newSealed)
				return
			case chain.MsgBlockSealed:
				sealed := chain.ParseSealedBlock(msg[1:])
				if sealed != nil {
					newSealed <- sealed
				}
			case chain.MsgCommitBlock:
				// blockchain will commit by itself.
			case chain.MsgBlockCommitted:
				committed := chain.ParseCommitBlock(msg[1:])
				if committed != nil {
					sealed := committed.Sealed()
					if sealed != nil {
						newSealed <- sealed
					}
				}
			}
		}
	}()

	go func() {
		cancel := ctx.Done()
		for {
			select {
			case <-cancel:
				conn.Shutdown()
				return
			case <-activate:
				
			case sealed := <-newSealed:
				if sealed == nil {
					continue
				}
				if len(sealed.Header.Candidate) > 0 {
					for _, candidate := range sealed.Header.Candidate {
						if candidate.Naked {
							naked[candidate.Node] = candidate
						} else {
							dressed[candidate.Node] = candidate
						}
					}
				}
				node.blockchain.AddSealedBlock(sealed)
				if node.blockchain.LastCommitEpoch == nextChecksumEpoch {
					// TODO
				} else if node.blockchain.LastCommitEpoch == nextWindowEpoch {
					// TODO

				} else if node.blockchain.LastCommitEpoch == nextStatementEpoch {
						tokens := GetConsensusTokens(naked, dressed, node.committee.weights, node.blockchain.LastCommitEpoch)
						if len(tokens) > 0 {
							committee := node.swellConfig.Permission.DeterminePool(node.blockchain, tokens)
							for _, member := range committee {
								if statement, ok := dressed[member.Token]; ok {
									member.Address = statement.Address
								}
							}
						} else {
							slog.Warn("RunCandidateNode: could not find consensus committee")
						}
					}
				}
			}
		}
	}()
	return validator
}

func ReconnectToAnotherNode(lost crypto.Token, available Valiadors, epoch uint64) *socket.SignedConnection {
	
	return nil
}
