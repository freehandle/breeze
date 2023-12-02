package swell

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type SwellNode struct {
	clockEpoch  uint64
	validators  Validators
	credentials crypto.PrivateKey
	blockchain  *chain.Blockchain
	actions     *store.ActionStore
	config      SwellNetworkConfiguration
	validating  []uint64 // windows
	isIdle      bool
}

func (s *SwellNode) AddSealedBlock(sealed *chain.SealedBlock) {
	s.blockchain.AddSealedBlock(sealed)
}

func (s *SwellNode) PurgeActions(actions *chain.ActionArray) {
	for n := 0; n < actions.Len(); n++ {
		hash := crypto.Hasher(actions.Get(n))
		s.actions.Exlude(hash)
	}
}

func (s *SwellNode) PurgeAction(hash crypto.Hash) {
	s.actions.Exlude(hash)
}

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

		for {
			select {
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
							finished = true
							cancelFunc()
						}
					} else if s.blockchain.LastCommitEpoch == nextStatementEpoch {
						tokens := GetConsensusTokens(naked, dressed, s.committee.weights, s.blockchain.LastCommitEpoch)
						if len(tokens) > 0 {
							committee := s.config.Permission.DeterminePool(s.blockchain, tokens)
							for _, member := range committee {
								if statement, ok := dressed[member.Token]; ok {
									member.Address = statement.Address
								}
							}
							JoinCandidateNode(ctx, node, committee, []byte{}, int(nextWindowEpoch))
						} else {
							slog.Warn("RunCandidateNode: could not find consensus committee")
							validator <- nil
							return
						}
					}
				}
			}
		}

	}()

}

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
