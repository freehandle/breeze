package poa

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

func NewValidator(blockchain *chain.Blockchain, config SingleAuthorityConfig) chan error {

	ctx := context.Background()

	finalize := make(chan error, 2)

	incomming, err := socket.Listen(fmt.Sprintf("localhost:%v", config.IncomingPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", config.IncomingPort, err)
		return finalize
	}

	outgoing, err := socket.Listen(fmt.Sprintf("localhost:%v", config.OutgoingPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", config.OutgoingPort, err)
		return finalize
	}

	actions := store.NewActionStore(ctx, blockchain.LastCommitEpoch)

	endIncomming := make(chan crypto.Token)
	newIncoming := make(chan *socket.SignedConnection)
	incomingConnections := make(map[crypto.Token]*socket.SignedConnection)

	newOutgoing := make(chan OutgoindConnectionRequest)

	action := actions.Pop

	incorporated := make(chan []byte)
	newBlock := make(chan *NewBlock)
	pool := make(socket.ConnectionPool)
	// listen incomming
	blockchain.NextBlock(1)
	go func() {
		for {
			if conn, err := incomming.Accept(); err == nil {
				trustedConn, err := socket.PromoteConnection(conn, config.Credentials, config.ValidateIncoming)
				if err != nil {
					conn.Close()
				}
				newIncoming <- trustedConn
			}
		}
	}()

	// manage incoming connections and block formation
	go func() {
		for {
			select {
			case token := <-endIncomming:
				delete(incomingConnections, token)
			case conn := <-newIncoming:
				incomingConnections[conn.Token] = conn
				go WaitForProtocolActions(conn, endIncomming, action)
			}
		}
	}()

	go func() {
		for {
			select {
			case newBlock := <-newBlock:
				header := messages.NewBlockMessage(newBlock.NewHeader.Serialize())
				if header != nil {
					pool.Broadcast(header)
				}
				seal := messages.BlockSealMessage(newBlock.NewHeader.Epoch-1, newBlock.OldSeal.Serialize())
				if seal != nil {
					pool.Broadcast(seal)
				}
				for _, old := range newBlock.OldCommit {
					msg := messages.CommitBlock(old.Epoch, old.Commit.Serialize())
					if msg != nil {
						pool.Broadcast(msg)
					}
				}
			case action := <-incorporated:
				data := append([]byte{messages.MsgAction}, action...)
				pool.Broadcast(data)
			case req := <-newOutgoing:
				cached := socket.NewCachedConnection(req.conn)
				pool.Add(cached)
				go blockchain.SyncBlocksServer(cached, req.epoch)
			}
		}

	}()

	// listen outgoing (cached with recent blocks)
	go func() {
		for {
			if conn, err := outgoing.Accept(); err == nil {
				trustedConn, err := socket.PromoteConnection(conn, config.Credentials, config.ValidateOutgoing)
				if err != nil {
					conn.Close()
				}
				go WaitForOutgoingSyncRequest(trustedConn, newOutgoing)
			} else {
				slog.Warn("poa outgoing listener error", "error", err)
				finalize <- fmt.Errorf("could not accept outgoing connection: %v", err)
				return
			}
		}
	}()

	return finalize
}
