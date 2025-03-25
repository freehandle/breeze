package swell

import (
	"context"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
)

type ReaderShutdowner interface {
	Read() ([]byte, error)
	Shutdown()
}

// ReadMessages reads block event from the provided connection and returns a
// channel for new sealed blocks derived from those events. The go-routine
// terminates either by the context, if there is an error on the connection or
// if it receives a MsgSyncError message (which is uses by the validator on the
// other side of the connection to tell that it is not capable of providing the
// requested information).
func ReadMessages(cancel context.CancelFunc, conn ReaderShutdowner) chan *chain.SealedBlock {
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
				// on swell each blockchain will commit by itself.
			case messages.MsgCommittedBlock:
				// extract the sealed block from the committed block
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
