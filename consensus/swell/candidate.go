package swell

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

const CandidateMsg byte = 200

// getConsensus tries to fiend a 2/3 + 1 consensus over a hash among members
func getConsensusHash(naked map[crypto.Token]*chain.ChecksumStatement, members map[crypto.Token]int) (crypto.Hash, bool) {
	totalweight := 0
	for _, weight := range members {
		totalweight += weight
	}
	weightPerHash := make(map[crypto.Hash]int)
	for _, statement := range naked {
		weight := weightPerHash[statement.Hash] + members[statement.Node]
		weightPerHash[statement.Hash] = weight
		if weight > 2*totalweight/3 {
			return statement.Hash, true
		}
	}
	return crypto.ZeroHash, false
}

// GetConsensusTokens checks if there is a naked hash with weight greater than
// 2/3 among current members of the validator pool. If so, it returns the tokens
// of all candidates that have provided naked and dressed statements for that
// hash.
func GetConsensusTokens(naked, dressed map[crypto.Token]*chain.ChecksumStatement, members map[crypto.Token]int, epoch uint64) []crypto.Token {
	hash, ok := getConsensusHash(naked, members)
	if !ok {
		return nil
	}
	tokens := make([]crypto.Token, 0)
	for _, nake := range naked {
		if nake.Hash.Equal(hash) {
			if dress, ok := dressed[nake.Node]; ok {
				if dress.IsDressed(nake) {
					tokens = append(tokens, nake.Node)
				}
			}
		}
	}
	return tokens
}

func RunCandidateNode(ctx context.Context, node *ValidatingNode, conn *socket.SignedConnection, iddle bool) chan *ValidatingNode {
	validator := make(chan *ValidatingNode)

	nextChecksumEpoch := node.blockchain.Checksum.Epoch + uint64(node.swellConfig.ChecksumWindow)
	lastChecksumEpoch := node.blockchain.Checksum.Epoch

	nextWindowEpoch := nextChecksumEpoch + uint64(node.swellConfig.ChecksumWindow)/2

	nextStatementEpoch := nextWindowEpoch - uint64(node.swellConfig.ChecksumWindow)/10

	newCtx, cancelFunc := context.WithCancel(ctx)
	newSealed := make(chan *chain.SealedBlock)

	go func() {
		for {
			msg, err := conn.Read()
			if err != nil {
				close(validator)
				return
			}
			if len(msg) < 1 {
				continue
			}
			switch msg[0] {
			case chain.MsgSyncError:
				close(validator)
				conn.Shutdown()
				cancelFunc()
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
						node.blockchain.AddSealedBlock(sealed)
					}
					sealedAfterChekcsum = nil
				}
			case sealed := <-newSealed:
				if !finished {
					if sealed != nil && sealed.Header.Epoch >= lastChecksumEpoch {
						if len(sealed.Header.Candidate) > 0 {
							for _, candidate := range sealed.Header.Candidate {
								if candidate.Naked {
									naked[candidate.Node] = candidate
								} else {
									dressed[candidate.Node] = candidate
								}
							}
						}
						if hasChecksum || sealed.Header.Epoch <= nextChecksumEpoch {
							node.blockchain.AddSealedBlock(sealed)
						} else {
							sealedAfterChekcsum = append(sealedAfterChekcsum, sealed)
						}
					}
					if node.blockchain.LastCommitEpoch == nextChecksumEpoch {
						node.blockchain.MarkCheckpoint(doneCheckppoint)
						hasRequestedChecksum = true
					} else if node.blockchain.LastCommitEpoch == nextWindowEpoch {
						finished = true
						cancelFunc()
					} else if node.blockchain.LastCommitEpoch == nextStatementEpoch {
						tokens := GetConsensusTokens(naked, dressed, node.committee.weights, node.blockchain.LastCommitEpoch)
						if len(tokens) > 0 {
							committee := node.swellConfig.Permission.DeterminePool(node.blockchain, tokens)
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
	return validator
}

func JoinCandidateNode(ctx context.Context, node *ValidatingNode, validators Validators, seed []byte, window int) {
	node.committee = LaunchValidatorPool(validators, node.credentials, seed)
	if node.committee == nil {
		slog.Info("JoinCandidateNode: could not form validator pool")
		return
	}
	RunValidatorNode(ctx, node, window)
}

/*
	type ValidatorCandidate struct {
		Epoch     uint64
		Token     crypto.Token
		Proof     crypto.Hash
		Signature crypto.Signature
	}

	func AttestCandidate(data []byte, checksum crypto.Hash, checksumEpoch uint64) bool {
		var epoch uint64
		var token crypto.Token
		var proof crypto.Hash
		var signature crypto.Signature

		if len(data) == 0 || data[0] != CandidateMsg {
			return false
		}
		position := 1
		epoch, position = util.ParseUint64(data, position)
		if epoch != checksumEpoch {
			return false
		}
		token, position = util.ParseToken(data, position)
		proof, position = util.ParseHash(data, position)
		if !proof.Equal(crypto.Hasher(append(token[:], checksum[:]...))) {
			return false
		}
		signature, _ = util.ParseSignature(data, position)
		return token.Verify(data[:position], signature)
	}

	func Candidate(credentials crypto.PrivateKey, epoch uint64, checksum crypto.Hash) []byte {
		bytes := []byte{CandidateMsg}
		token := credentials.PublicKey()
		util.PutUint64(epoch, &bytes)
		util.PutToken(token, &bytes)
		util.PutHash(crypto.Hasher(append(token[:], checksum[:]...)), &bytes)
		signature := credentials.Sign(bytes)
		util.PutSignature(signature, &bytes)
		return bytes
	}
*/
