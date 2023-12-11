package swell

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

// RunNonValidatingNode engine runs the underlying swell node as a non validating
// node. This means that it is receiving block events from a connection to a
// relay network from one or more validators. If candidate is true the node will
// try to become a validator for the next window. The responsability for the
// permission to become a validator is left to the holder of the node credentials.
func (s *SwellNode) RunNonValidatingNode(ctx context.Context, conn *socket.SignedConnection, candidate bool) {

	nextChecksumEpoch := s.blockchain.Checksum.Epoch + uint64(s.config.ChecksumWindow)
	lastChecksumEpoch := s.blockchain.Checksum.Epoch
	nextWindowEpoch := nextChecksumEpoch + uint64(s.config.ChecksumWindow)/2
	nextStatementEpoch := nextWindowEpoch - uint64(s.config.ChecksumWindow)/10

	newCtx, cancelFunc := context.WithCancel(ctx)

	newSealed := ReadMessages(cancelFunc, conn)

	go func() {
		hasChecksum := false
		hasRequestedChecksum := false
		sealedAfterChekcsum := make([]*chain.SealedBlock, 0)
		doneCheckppoint := make(chan bool)
		cancel := newCtx.Done()
		finished := false
		dressed := make(map[crypto.Token]*chain.ChecksumStatement)
		naked := make(map[crypto.Token]*chain.ChecksumStatement)
		var activateResponse chan error
		for {
			select {
			case req := <-s.relay.SyncRequest:
				fmt.Println("sync request")
				if req.State {
					go s.blockchain.SyncState(req.Conn)
				} else {
					go s.blockchain.SyncBlocksServer(req.Conn, req.Epoch)
				}
				fmt.Println("sync request", req.Epoch, req.State)
			case activateResponse = <-s.active:
				candidate = true
			case <-cancel:
				conn.Shutdown()
				if hasRequestedChecksum && !hasChecksum {
					<-doneCheckppoint
					close(doneCheckppoint)
				} else {
					close(doneCheckppoint)
				}
				return
			case ok := <-doneCheckppoint:
				if !ok {
					cancelFunc()
				} else {
					hasChecksum = true
					fmt.Println("checksum", s.blockchain.Checksum.Epoch, crypto.EncodeHash(s.blockchain.Checksum.Hash))
					for _, sealed := range sealedAfterChekcsum {
						s.blockchain.AddSealedBlock(sealed)
					}
					sealedAfterChekcsum = nil
				}
			case sealed := <-newSealed:
				if !finished { // not responsible anymore
					if sealed != nil && sealed.Header.Epoch >= lastChecksumEpoch {
						// keep information to determine next pool
						if len(sealed.Header.Candidate) > 0 {
							for _, candidate := range sealed.Header.Candidate {
								if candidate.Naked {
									naked[candidate.Node] = candidate
								} else {
									dressed[candidate.Node] = candidate
								}
							}
						}
						// delay adding sealed blocks after checksum epoch after in possession of checksum.
						if hasChecksum || sealed.Header.Epoch <= nextChecksumEpoch {
							s.blockchain.AddSealedBlock(sealed)
						} else {
							sealedAfterChekcsum = append(sealedAfterChekcsum, sealed)
						}
					}
					if s.blockchain.LastCommitEpoch == nextChecksumEpoch {
						s.blockchain.MarkCheckpoint(doneCheckppoint)
						hasRequestedChecksum = true
					} else if s.blockchain.LastCommitEpoch == nextWindowEpoch {
						if candidate {
							// ValidatorNode wiill be responsible for this window
							if activateResponse != nil {
								activateResponse <- nil
							}
							finished = true
							cancelFunc()
						}
					} else if s.blockchain.LastCommitEpoch == nextStatementEpoch {
						tokens := GetConsensusTokens(naked, dressed, s.validators.weights, s.blockchain.LastCommitEpoch)
						if len(tokens) > 0 {
							approved := s.config.Permission.DeterminePool(s.blockchain, tokens)
							committee := make(Validators, 0)
							for token, weight := range approved {
								if statement, ok := dressed[token]; ok {
									member := Validator{
										Address: statement.Address,
										Weight:  weight,
										Token:   token,
									}
									committee = append(committee, &member)
								}
							}
							s.JoinCandidateNode(ctx, committee, []byte{}, int(nextWindowEpoch))
						} else {
							slog.Warn("RunCandidateNode: could not find consensus committee")
						}
					}
				}
			}
		}
	}()
}

// JoinCandidateNode lauches a validator pool of connections with other
// accredited validators. And runs the swell node by the ValidatingNode engine.
// Notice that at the same time more than one engine can be active on the same
// swell node. JoinCandidateNode is called befero the ending of the current
// checksum window, in order to give time for the proper network formation.
func (s *SwellNode) JoinCandidateNode(ctx context.Context, validators Validators, seed []byte, window int) {
	committee := LaunchValidatorPool(validators, s.credentials, seed)
	if committee == nil {
		slog.Warn("JoinCandidateNode: could not launch validator pool")
		return
	}
	s.RunValidatingNode(ctx, committee, window)
}

// ReadMessages reads block event from the provided connection and returns a
// channel for new sealed blocks derived from those events. The go-routine
// terminates either by the context, if there is an error on the connection or
// if it receives a MsgSyncError message (which is uses by the validator on the
// other side of the connection to tell that it is not capable of providing the
// requested information).
func ReadMessages(cancel context.CancelFunc, conn *socket.SignedConnection) chan *chain.SealedBlock {
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
				cancel()
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
	return newSealed
}
