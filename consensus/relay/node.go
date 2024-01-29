/*
Package relay provides an external interface for a validating node.
The relay opens a port for actions gateway, another port that listen to
requests to receive block events and a third port for administrative
purposes for the node.
*/
package relay

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"time"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

type Shutdowner interface {
	Shutdown()
}

// Config defines the configuration for a relay node. The firewall defines the
// authorized connections for the gateway and the block listener. The credentials
// should be the private key of the validating node. Hostname is "localhost" or
// empty for internet connections. For test it can be any string.
type Config struct {
	GatewayPort       int
	BlockListenerPort int
	Firewall          *Firewall
	Credentials       crypto.PrivateKey
	Hostname          string
}

// NewFireWall returns a new firewall with the authorized gateway and block listener
// tokens.
func NewFireWall(authorizedGateway []crypto.Token, autorizedBlockListener []crypto.Token, openGateway, opeanBlockListener bool) *Firewall {
	return &Firewall{
		AcceptGateway:       socket.NewValidConnections(authorizedGateway, openGateway),
		AcceptBlockListener: socket.NewValidConnections(autorizedBlockListener, opeanBlockListener),
	}

}

// Firewall defines the authorized connections for the gateway and the block listener.
type Firewall struct {
	AcceptGateway       *socket.AcceptValidConnections
	AcceptBlockListener *socket.AcceptValidConnections
}

// Node defines the external interface for a validating node. ActionGateway
// channel shoud be read by the validating node to receive proposed actions.
// BlockEvents channel should be write by the validating node to broadcast block
// events. SyncRequest channel should be read by the validating node to receive
// requests for state sync and recenet blocks sync.
type Node struct {
	ActionGateway      chan []byte                   // Sends actions to swell engine
	BlockEvents        chan []byte                   // receive block events from swell engine
	SyncRequest        chan SyncRequest              // sends sync requests to swell engine
	TopologyRequest    chan *socket.SignedConnection // sends request for topology
	Statement          chan []byte
	config             *Config
	gatewayConnections map[crypto.Token]*socket.SignedConnection
	pool               socket.ConnectionPool
}

func (n *Node) Status() string {
	status := ""
	if len(n.gatewayConnections) > 0 {
		status = fmt.Sprintf("%vthere are %v gateway connected\n", status, len(n.gatewayConnections))
	} else {
		status = fmt.Sprintf("%vthere is no gateway connected\n", status)
	}
	if len(n.pool) > 0 {
		status = fmt.Sprintf("%vthere are %v block listener connected\n", status, len(n.pool))
	} else {
		status = fmt.Sprintf("%vthere is no block listener connected\n", status)
	}
	if firewall := n.config.Firewall; firewall != nil {
		if gateway := firewall.AcceptGateway; gateway != nil {
			status = fmt.Sprintf("%vgateway rule: %v\n", status, gateway.String())
		}
		if blocks := firewall.AcceptBlockListener; blocks != nil {
			status = fmt.Sprintf("%vblock listener rule: %v\n", status, blocks.String())
		}
	}
	return status
}

// SyncRequest defines a request for state sync and recent blocks sync. Epoch
// is the first epoch for which the requester needs a block. State is true if
// the requester needs a state sync.
type SyncRequest struct {
	Epoch uint64
	State bool
	Conn  *socket.CachedConnection
}

// Run starts a relay network. It returns a Node and an error. On cancelation
// of the context, the entire relay network is graciously shutdown.
func Run(ctx context.Context, cfg *Config) (*Node, error) {
	n := &Node{
		ActionGateway:   make(chan []byte),
		Statement:       make(chan []byte),
		BlockEvents:     make(chan []byte),
		SyncRequest:     make(chan SyncRequest),
		TopologyRequest: make(chan *socket.SignedConnection),
		config:          cfg,
	}

	var listenAdminPort net.Listener

	gatewayPort, err := socket.Listen(fmt.Sprintf("%v:%v", n.config.Hostname, n.config.GatewayPort))
	if err != nil {
		return nil, fmt.Errorf("could not listen on port %v: %v", n.config.GatewayPort, err)
	}

	blocksPort, err := socket.Listen(fmt.Sprintf("%v:%v", n.config.Hostname, n.config.BlockListenerPort))
	if err != nil {
		return nil, fmt.Errorf("could not listen on port %v: %v", n.config.BlockListenerPort, err)
	}

	endGateway := make(chan crypto.Token)
	newGateway := make(chan *socket.SignedConnection)
	n.gatewayConnections = make(map[crypto.Token]*socket.SignedConnection)

	newBlockListener := make(chan SyncRequest)
	action := make(chan []byte)
	cloned := make(chan bool)
	n.pool = make(socket.ConnectionPool)

	dropConnection := make(chan crypto.Token)

	// manage incoming connections and block formation
	go func() {
		defer func() {
			n.pool.DropAll()
			gatewayPort.Close()
			blocksPort.Close()
			if listenAdminPort != nil {
				listenAdminPort.Close()
			}
			for _, conn := range n.gatewayConnections {
				conn.Shutdown()
			}
		}()
		cancel := ctx.Done()
		for {
			select {
			case <-cancel:
				return
			case token := <-endGateway:
				delete(n.gatewayConnections, token)
			case conn := <-newGateway:
				if conn != nil {
					n.gatewayConnections[conn.Token] = conn
					go WaitForProtocolActions(conn, endGateway, action)
				}
			case proposed := <-action:
				if len(proposed) > 0 {
					if proposed[0] == messages.MsgChecksumStatement {
						n.Statement <- proposed[1:]
					} else {
						n.ActionGateway <- proposed[1:]
					}
				} else {
					slog.Error("relay.Run: empty action on channel")
				}
			case ok := <-cloned:
				log.Printf("state cloned: %v", ok)
			case blockEvent := <-n.BlockEvents:
				n.pool.Broadcast(blockEvent)
			case req := <-newBlockListener:
				n.pool.Add(req.Conn)
				if req.Epoch != 1<<64-1 {
					n.SyncRequest <- req
				}
			case token := <-dropConnection:
				n.pool.Drop(token)
			}
		}
	}()

	// listen incomming
	go func() {
		for {
			if conn, err := gatewayPort.Accept(); err == nil {
				var accept socket.ValidateConnection
				if n.config.Firewall != nil {
					accept = n.config.Firewall.AcceptGateway
				} else {
					accept = socket.AcceptAllConnections
				}
				trustedConn, err := socket.PromoteConnection(conn, n.config.Credentials, accept)
				if err != nil {
					conn.Close()
				}
				newGateway <- trustedConn
			}
		}
	}()

	// listen outgoing (cached with recent blocks)
	go func() {
		for {
			if conn, err := blocksPort.Accept(); err == nil {
				var accept socket.ValidateConnection
				if n.config.Firewall != nil {
					accept = n.config.Firewall.AcceptBlockListener
				} else {
					accept = socket.AcceptAllConnections
				}
				fmt.Println(n.config.Credentials, n.config.Credentials.PublicKey())
				trustedConn, err := socket.PromoteConnection(conn, n.config.Credentials, accept)
				if err != nil {
					conn.Close()
					continue
				}
				go WaitForOutgoingSyncRequest(trustedConn, newBlockListener, dropConnection, action, n.TopologyRequest)
			} else {
				slog.Warn("poa outgoing listener error", "error", err)
				return
			}
		}
	}()

	return n, nil
}

// WaitForProtocolActions reads proposed actions from a connection and sends them
// to the action channel. If the connection is terminated, it sends the connection
// token to the terminate channel.
func WaitForProtocolActions(conn *socket.SignedConnection, terminate chan crypto.Token, action chan []byte) {
	for {
		data, err := conn.Read()
		fmt.Println(data)
		if err != nil || len(data) < 2 || data[0] != messages.MsgActionSubmit {
			if err != nil {
				slog.Info("poa WaitForProtocolActions: connection terminated", "connection", err)
			} else {
				slog.Info("poa WaitForProtocolActions: invalid action", "connection", conn.Token, "data", data)
			}
			conn.Shutdown()
			terminate <- conn.Token
			return
		}
		if data[0] == messages.MsgChecksumStatement {
			fmt.Println("got checksum statement")
		}
		action <- data
	}
}

// WaitForOutgoingSyncRequest reads a sync request from a connection and sends
// it to the sync request channel. If it is not a valid request, if closes the
// conection and returns without sending anything to outgoing channel.
func WaitForOutgoingSyncRequest(conn *socket.SignedConnection, outgoing chan SyncRequest, drop chan crypto.Token, action chan []byte, topology chan *socket.SignedConnection) {
	if conn == nil {
		slog.Error("relay node synchronization: nil connection")
		return
	}
	lastSync := time.Now().Add(-time.Hour)
	for {
		data, err := conn.Read()
		fmt.Println(data)
		if err != nil || len(data) < 1 {
			if err != nil {
				slog.Info("relay node synchronization connection terminated", "node", conn.Token)
			} else {
				slog.Info("relay node synchronization: invalid sync request message", "node", conn.Token, "message code", data[0], "message length", len(data))
			}
			drop <- conn.Token
			return
		}
		if data[0] == messages.MsgSyncRequest {
			if time.Since(lastSync) > time.Minute {
				lastSync = time.Now()
				epoch, position := util.ParseUint64(data, 1)
				state, _ := util.ParseBool(data, position)
				cached := socket.NewCachedConnection(conn)
				outgoing <- SyncRequest{Conn: cached, Epoch: epoch, State: state}
			}
		} else if data[0] == messages.MsgChecksumStatement {
			action <- data
		} else if data[0] == messages.MsgNetworkTopologyReq {
			fmt.Println("topoloty requyest")
			topology <- conn
		} else if data[0] == messages.MsgSubscribeBlockEvents {
			cached := socket.NewCachedConnection(conn)
			cached.Ready()
			outgoing <- SyncRequest{Conn: cached, Epoch: 1<<64 - 1}
		} else {
			conn.Send([]byte{messages.MsgError})
		}
	}
}
