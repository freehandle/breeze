package poa

import (
	"fmt"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

func NewValidator(blockchain *chain.Chain, config SingleAuthorityConfig) chan error {

	finalize := make(chan error, 2)

	incomming, err := socket.Default.Listen("tcp", fmt.Sprintf(":%v", config.IncomingPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", config.IncomingPort, err)
		return finalize
	}

	outgoing, err := socket.Default.Listen("tcp", fmt.Sprintf(":%v", config.OutgoingPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", config.OutgoingPort, err)
		return finalize
	}

	actions := store.NewActionStore(blockchain.LastCommitEpoch)

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
				header := chain.NewBlockMessage(newBlock.NewHeader)
				if header != nil {
					pool.Broadcast(header)
				}
				seal := chain.BlockSealMessage(newBlock.NewHeader.Epoch-1, newBlock.OldSeal)
				if seal != nil {
					pool.Broadcast(seal)
				}
				for _, old := range newBlock.OldCommit {
					msg := chain.CommitBlockMessage(old.Epoch, old.Commit)
					if msg != nil {
						pool.Broadcast(msg)
					}
				}
			case action := <-incorporated:
				data := append([]byte{chain.MsgAction}, action...)
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