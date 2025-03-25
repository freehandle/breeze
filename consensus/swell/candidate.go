package swell

import (
	"context"
	"log/slog"
	"math/rand"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

const CandidateMsg byte = 200

func ConnectRandomValidator(hostname string, credentials crypto.PrivateKey, validators []socket.TokenAddr) *socket.SignedConnection {
	value := rand.Intn(len(validators))
	for n := 0; n < len(validators); n++ {
		selected := validators[(n+value)%len(validators)]
		conn, err := socket.Dial(hostname, selected.Addr, credentials, selected.Token)
		if err == nil {
			return conn
		}
	}
	// could not connect to any
	return nil
}

// StartNonValidatorEngine engine runs the underlying swell node as a non validating
// node. This means that it is receiving block events from a connection to a
// relay network from one or more validators. If candidate is true the node will
// try to become a validator for the next window. The responsability for the
// permission to become a validator is left to the holder of the node credentials.
func StartNonValidatorEngine(w *Window, conn *socket.SignedConnection, candidate bool) context.CancelFunc {
	newCtx, cancelFunc := context.WithCancel(w.ctx)
	newSealed := ReadMessages(cancelFunc, conn)
	go func() {
		canceled := newCtx.Done()
		finished := false
		var activateResponse chan error
		for {
			select {
			case activateResponse = <-w.Node.active:
				slog.Info("RunNonValidatorNode: request activation", "window start", w.Start)
				candidate = true
			case <-canceled:
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
	return cancelFunc
}
