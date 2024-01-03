package index

import (
	"fmt"
	"log"
	"net"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
	"github.com/freehandle/cb/topos"
)

// RelayConfig is the configuration for a relay node.
type DBConfig struct {
	SourceAddress string                    // url:port
	SourceToken   crypto.Token              // known token of the block provider
	Credentials   crypto.PrivateKey         // secret key of the relay node
	ListenPort    int                       // other nodes must connect to this port
	Validate      socket.ValidateConnection // check if a token is allowed
	HashFunc      func([]byte) []crypto.Hash
}

type DBSyncRequest struct {
	Conn   *socket.CachedConnection
	Hashes []crypto.Hash
	Epoch  uint64
}

type ActionWithHashes struct {
	Epoch    uint64
	Sequence int64
	Hashes   []crypto.Hash
}

func NewDB(config DBConfig, index *Index, chain *topos.Blockchain) chan error {

	finalize := make(chan error, 2)

	listeners, err := net.Listen("tcp", fmt.Sprintf(":%v", config.ListenPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", config.ListenPort, err)
		return finalize
	}

	conn, err := socket.Dial("localhost", config.SourceAddress, config.Credentials, config.SourceToken)
	if err != nil {
		finalize <- fmt.Errorf("could not connect to block provider: %v", err)
		return finalize
	}

	if err := conn.Send(topos.NewSyncRequest(index.lastEpoch)); err != nil {
		finalize <- fmt.Errorf("could not send sync request: %v", err)
		return finalize
	}

	incorporate := make(chan *topos.RelaySyncRequest)
	msg := make(chan []byte)

	pool := make(socket.ConnectionPool)

	go func() {
		for {
			data, err := conn.Read()
			if err != nil {
				finalize <- fmt.Errorf("error reading from block provider: %v", err)
				return
			}
			if data[0] == topos.MsgBlock {
				if len(data) == 1+8+crypto.Size {
					epoch, _ := util.ParseUint64(data, 1)
					index.NextBlock(epoch)
				} else {
					log.Print("invalid new block message")
				}
			} else if data[0] == topos.MsgAction {
				if len(data) > 1 {
					action := data[1:]
					//hashes := config.HashFunc(action)
					if err := chain.Append(action); err != nil {
						log.Printf("invalid action: %v", err)
					} else {
						msg <- data
					}
				} else {
					log.Print("invalid action message")
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case request := <-incorporate:
				if request == nil {
					return
				}
				pool[request.Token] = request.Conn
				go chain.Sync(request.Conn, request.Epoch)
			case data := <-msg:
				pool.Broadcast(append([]byte{topos.MsgBlock}, data...))
			}
		}
	}()

	go func() {
		for {
			if conn, err := listeners.Accept(); err == nil {
				trustedConn, err := socket.PromoteConnection(conn, config.Credentials, config.Validate)
				if err != nil {
					conn.Close()
				} else {

					go topos.WaitSyncRequest(trustedConn, incorporate)
				}
			} else {
				return
			}
		}
	}()
	return finalize
}
