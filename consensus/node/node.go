package node

import (
	"fmt"
	"log"
	"log/slog"
	"net"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

type Config struct {
	GatewayPort     int
	ValidateGateway []crypto.Token
	BlocksPort      int
	ValidateBlocks  []crypto.Token
	AdminPort       int
	Credentials     crypto.PrivateKey
}

type Node struct {
	ActionGateway chan []byte      // Sends actions to swell engine
	BlockEvents   chan []byte      // receive block events from swell engine
	SyncRequest   chan SyncRequest // sends sync requests to swell engine
}

type SyncRequest struct {
	Epoch uint64
	State bool
	Conn  *socket.CachedConnection
}

func NewNode() *Node {
	return &Node{
		ActionGateway: make(chan []byte),
		BlockEvents:   make(chan []byte),
		SyncRequest:   make(chan SyncRequest),
	}
}

func (n *Node) Run(config Config) chan error {
	finalize := make(chan error, 2)

	gatewayPort, err := net.Listen("tcp", fmt.Sprintf(":%v", config.GatewayPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", config.GatewayPort, err)
		return finalize
	}

	blocksPort, err := net.Listen("tcp", fmt.Sprintf(":%v", config.BlocksPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", config.BlocksPort, err)
		return finalize
	}

	if config.AdminPort > 0 {
		listenAdminPort, err := net.Listen("tcp", fmt.Sprintf(":%v", config.AdminPort))
		if err != nil {
			finalize <- fmt.Errorf("could not listen on port %v: %v", config.BlocksPort, err)
			return finalize
		}
		go func() {
			validator := socket.ValidateSingleConnection(config.Credentials.PublicKey())
			for {
				if conn, err := listenAdminPort.Accept(); err == nil {
					trustedConn, err := socket.PromoteConnection(conn, config.Credentials, validator)
					if err != nil {
						conn.Close()
					}
					go AdminConnection(trustedConn * socket.SignedConnection)
				}
			}
		}()
	}

	endGateway := make(chan crypto.Token)
	newGateway := make(chan *socket.SignedConnection)
	gatewayConnections := make(map[crypto.Token]*socket.SignedConnection)

	newBlockListener := make(chan SyncRequest)
	action := make(chan []byte)
	cloned := make(chan bool)
	pool := make(socket.ConnectionPool)

	// listen incomming
	go func() {
		for {
			if conn, err := gatewayPort.Accept(); err == nil {
				trustedConn, err := socket.PromoteConnection(conn, config.Credentials, config.ValidateGateway)
				if err != nil {
					conn.Close()
				}
				newGateway <- trustedConn
			}
		}
	}()

	// manage incoming connections and block formation
	go func() {
		for {
			select {
			case token := <-endGateway:
				delete(gatewayConnections, token)
			case conn := <-newGateway:
				gatewayConnections[conn.Token] = conn
				go WaitForProtocolActions(conn, endGateway, action)
			case proposed := <-action:
				n.ActionGateway <- proposed
			case ok := <-cloned:
				log.Printf("state cloned: %v", ok)
			case blockEvent := <-n.BlockEvents:
				pool.Broadcast(blockEvent)
			case req := <-newBlockListener:
				pool.Add(req.Conn)
				n.SyncRequest <- req
			}
		}
	}()

	// listen outgoing (cached with recent blocks)
	go func() {
		for {
			if conn, err := blocksPort.Accept(); err == nil {
				trustedConn, err := socket.PromoteConnection(conn, config.Credentials, config.ValidateGateway)
				if err != nil {
					conn.Close()
				}
				go WaitForOutgoingSyncRequest(trustedConn, newBlockListener)
			} else {
				slog.Warn("poa outgoing listener error", "error", err)
				finalize <- fmt.Errorf("could not accept outgoing connection: %v", err)
				return
			}
		}
	}()

	return finalize
}

// WaitForProtocolActions reads proposed actions from a connection and sends them
// to the action channel. If the connection is terminated, it sends the connection
// token to the terminate channel.
func WaitForProtocolActions(conn *socket.SignedConnection, terminate chan crypto.Token, action chan []byte) {
	for {
		data, err := conn.Read()
		if err != nil || len(data) < 2 || data[0] != chain.MsgActionSubmit {
			if err != nil {
				slog.Info("poa WaitForProtocolActions: connection terminated", "connection", err)
			} else {
				slog.Info("poa WaitForProtocolActions: invalid action", "connection", conn.Token)
			}
			conn.Shutdown()
			terminate <- conn.Token
			return
		}
		action <- data[1:]
	}
}

func WaitForOutgoingSyncRequest(conn *socket.SignedConnection, outgoing chan SyncRequest) {
	data, err := conn.Read()
	if err != nil || len(data) != 9 || data[0] != chain.MsgSyncRequest {
		if err != nil {
			slog.Info("poa WaitForOutgoingSyncRequest: connection terminated", "connection", err)
		} else {
			slog.Info("poa WaitForOutgoingSyncRequest: invalid sync request", "connection", conn.Token)
		}
		conn.Shutdown()
		return
	}
	epoch, position := util.ParseUint64(data, 1)
	state, _ := util.ParseBool(data, position)
	cached := socket.NewCachedConnection(conn)
	outgoing <- SyncRequest{Conn: cached, Epoch: epoch, State: state}
}

func AdminConnection(conn *socket.SignedConnection) {