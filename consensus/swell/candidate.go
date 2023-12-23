package swell

import (
	"context"
	"log/slog"
	"math/rand"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

const CandidateMsg byte = 200

func ConnectRandomValidator(hostname string, credentials crypto.PrivateKey, validators Validators) *socket.SignedConnection {
	value := rand.Intn(len(validators))
	for n := 0; n < len(validators); n++ {
		selected := validators[(n+value)%len(validators)]
		conn, err := socket.Dial(hostname, selected.Address, credentials, selected.Token)
		if err == nil {
			return conn
		}
	}
	// could not connect to any
	return nil
}

// RunNonValidatingNode engine runs the underlying swell node as a non validating
// node. This means that it is receiving block events from a connection to a
// relay network from one or more validators. If candidate is true the node will
// try to become a validator for the next window. The responsability for the
// permission to become a validator is left to the holder of the node credentials.
func RunNonValidatorNode(w *Window, conn *socket.SignedConnection, candidate bool) {

	//window := s.blockchain.Checksum.Epoch / s.config.ChecksumWindow
	//epochs := GetChecksumWindowEpochs(s.blockchain.Checksum.Epoch, s.config.ChecksumWindow)
	/*nextChecksumWindow := s.blockchain.Checksum.Epoch + uint64(s.config.ChecksumWindow)
	lastChecksumWindow := s.blockchain.Checksum.Epoch
	nextWindowEpoch := nextChecksumWindow - uint64(s.config.ChecksumWindow)/2
	nextStatementEpoch := nextChecksumWindow - uint64(s.config.ChecksumWindow)/10
	*/

	newCtx, cancelFunc := context.WithCancel(w.ctx)
	newSealed := ReadMessages(cancelFunc, conn)
	go func() {
		cancel := newCtx.Done()
		finished := false
		var activateResponse chan error
		for {
			select {
			case activateResponse = <-w.Node.active:
				slog.Info("RunNonValidatorNode: request activation", "window start", w.Start)
				candidate = true
			case <-cancel:
				conn.Shutdown()
				return
			case sealed := <-newSealed:
				if !finished { // not responsible anymore
					if sealed != nil {
						w.AddSealedBlock(sealed)
					}
					if w.Finished() {
						if candidate {
							// ValidatorNode wiill be responsible for this window
							if activateResponse != nil {
								activateResponse <- nil
							}
							finished = true
							cancelFunc()
						}
					} else if statement := w.DressedChecksumStatement(w.Node.blockchain.LastCommitEpoch); statement != nil {
						slog.Info("Window: non validator candidate node sending dressed checksum", "window start", w.Start, "checksum", crypto.EncodeHash(statement.Hash))
						conn.Send(append([]byte{messages.MsgChecksumStatement}, statement.Serialize()...))
					} else if statement := w.NakedChecksumWindow(w.Node.blockchain.LastCommitEpoch); statement != nil {
						slog.Info("Window: non validator candidate node sending naked checksum", "window start", w.Start, "checksum", crypto.EncodeHash(statement.Hash))
						conn.Send(append([]byte{messages.MsgChecksumStatement}, statement.Serialize()...))
					}
				}
			}
		}
	}()
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
			case messages.MsgSyncError:
				conn.Shutdown()
				cancel()
				close(newSealed)
				return
			case messages.MsgSealedBlock:
				sealed := chain.ParseSealedBlock(msg[1:])
				if sealed != nil {
					newSealed <- sealed
				}
			case messages.MsgCommit:
				// blockchain will commit by itself.
			case messages.MsgCommittedBlock:
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
