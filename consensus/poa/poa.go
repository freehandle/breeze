package poa

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

const checksumBlockInterval = 15 * 1 // 1 minutes

type SingleAuthorityConfig struct {
	IncomingPort     int
	OutgoingPort     int
	Credentials      crypto.PrivateKey
	BlockInterval    time.Duration
	ValidateIncoming socket.ValidateConnection
	ValidateOutgoing socket.ValidateConnection
	WalletFilePath   string
	KeepBlocks       int
}

type OutgoindConnectionRequest struct {
	conn  *socket.SignedConnection
	epoch uint64
}

type NewBlock struct {
	NewHeader chain.BlockHeader
	OldSeal   chain.BlockSeal
	OldCommit map[uint64]*chain.BlockCommit
}

// Single Authorities listens to gateway port to receive instructions gateway providers
// and listens to subscriber port to broadcast blockchain information.
func Genesis(config SingleAuthorityConfig) chan error {

	finalize := make(chan error, 2)

	blockchain := chain.NewChainFromGenesisState(config.Credentials, config.WalletFilePath)
	if blockchain == nil {
		finalize <- errors.New("could not create genesis state")
		return finalize
	}
	blockchain.LastCommitHash = crypto.HashToken(config.Credentials.PublicKey())

	incomming, err := net.Listen("tcp", fmt.Sprintf(":%v", config.IncomingPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", config.IncomingPort, err)
		return finalize
	}

	outgoing, err := net.Listen("tcp", fmt.Sprintf(":%v", config.OutgoingPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", config.OutgoingPort, err)
		return finalize
	}

	endIncomming := make(chan crypto.Token)
	newIncoming := make(chan *socket.SignedConnection)
	incomingConnections := make(map[crypto.Token]*socket.SignedConnection)

	newOutgoing := make(chan OutgoindConnectionRequest)

	action := make(chan []byte)
	incorporated := make(chan []byte)
	newBlock := make(chan *NewBlock)

	ticker := time.NewTicker(config.BlockInterval)

	cloned := make(chan bool)

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
			case proposed := <-action:
				if ok := blockchain.Validate(proposed); ok {
					incorporated <- proposed
				} else {
				}
			case <-cloned:
				fmt.Println("cloned")
			case <-ticker.C:
				epoch := blockchain.LiveBlock.Header.Epoch
				blockchain.SealOwnBlock()
				sealed := blockchain.SealedBlocks[len(blockchain.SealedBlocks)-1]
				//fmt.Printf("%+v\n", blockchain.SealedBlocks[len(blockchain.SealedBlocks)-1])
				commit := make(map[uint64]*chain.BlockCommit)
				if !blockchain.Cloning {
					for e := blockchain.LastCommitEpoch + 1; e <= epoch; e++ {
						if !blockchain.CommitBlock(e, config.Credentials) {
							break // no more sealed blocks available
						}
						last := blockchain.RecentBlocks[len(blockchain.RecentBlocks)-1]
						commit[e] = last.Commit
					}
				}
				if epoch%checksumBlockInterval == 0 {
					blockchain.MarkCheckpoint(cloned)
				}
				blockchain.NextBlock(epoch + 1)
				newBlock <- &NewBlock{
					NewHeader: blockchain.LiveBlock.Header,
					OldSeal:   sealed.Seal,
					OldCommit: commit,
				}
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
				for epoch, commit := range newBlock.OldCommit {
					msg := chain.CommitBlockMessage(epoch, commit)
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
				trustedConn, err := socket.PromoteConnection(conn, config.Credentials, config.ValidateIncoming)
				if err != nil {
					conn.Close()
				}
				go WaitForOutgoingSyncRequest(trustedConn, newOutgoing)
			}

		}
	}()

	return finalize
}

func WaitForOutgoingSyncRequest(conn *socket.SignedConnection, outgoing chan OutgoindConnectionRequest) {
	data, err := conn.Read()
	fmt.Println("**********", data, chain.MsgSyncRequest)
	if err != nil || len(data) != 9 || data[0] != chain.MsgSyncRequest {
		conn.Shutdown()
		return
	}
	epoch, _ := util.ParseUint64(data, 1)
	outgoing <- OutgoindConnectionRequest{conn: conn, epoch: epoch}
}

func WaitForProtocolActions(conn *socket.SignedConnection, terminate chan crypto.Token, action chan []byte) {
	for {
		data, err := conn.Read()
		if err != nil || len(data) < 2 || data[0] != chain.MsgActionSubmit {
			conn.Shutdown()
			terminate <- conn.Token
			return
		}
		action <- data[1:]
	}
}
