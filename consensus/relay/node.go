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
	AdminPort         int
	Firewall          *Firewall
	Credentials       crypto.PrivateKey
	Hostname          string
}

// NewFireWall returns a new firewall with the authorized gateway and block listener
// tokens.
func NewFireWall(authorizedGateway []crypto.Token, autorizedBlockListener []crypto.Token) *Firewall {
	return &Firewall{
		AcceptGateway:       socket.NewValidConnections(authorizedGateway),
		AcceptBlockListener: socket.NewValidConnections(autorizedBlockListener),
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
	ActionGateway chan []byte      // Sends actions to swell engine
	BlockEvents   chan []byte      // receive block events from swell engine
	SyncRequest   chan SyncRequest // sends sync requests to swell engine
	Statement     chan []byte
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
func Run(ctx context.Context, config Config) (*Node, error) {
	n := &Node{
		ActionGateway: make(chan []byte),
		Statement:     make(chan []byte),
		BlockEvents:   make(chan []byte),
		SyncRequest:   make(chan SyncRequest),
	}

	var listenAdminPort net.Listener

	gatewayPort, err := socket.Listen(fmt.Sprintf("%v:%v", config.Hostname, config.GatewayPort))
	if err != nil {
		return nil, fmt.Errorf("could not listen on port %v: %v", config.GatewayPort, err)
	}

	blocksPort, err := socket.Listen(fmt.Sprintf("%v:%v", config.Hostname, config.BlockListenerPort))
	if err != nil {
		return nil, fmt.Errorf("could not listen on port %v: %v", config.BlockListenerPort, err)
	}

	endGateway := make(chan crypto.Token)
	newGateway := make(chan *socket.SignedConnection)
	gatewayConnections := make(map[crypto.Token]*socket.SignedConnection)

	newBlockListener := make(chan SyncRequest)
	action := make(chan []byte)
	cloned := make(chan bool)
	pool := make(socket.ConnectionPool)

	dropConnection := make(chan crypto.Token)

	adminConnections := make([]Shutdowner, 0)

	// manage incoming connections and block formation
	go func() {
		defer func() {
			pool.DropAll()
			gatewayPort.Close()
			blocksPort.Close()
			if listenAdminPort != nil {
				listenAdminPort.Close()
			}
			for _, conn := range gatewayConnections {
				conn.Shutdown()
			}
			for _, conn := range adminConnections {
				conn.Shutdown()
			}
		}()
		cancel := ctx.Done()
		for {
			select {
			case <-cancel:
				return
			case token := <-endGateway:
				delete(gatewayConnections, token)
			case conn := <-newGateway:
				if conn != nil {
					gatewayConnections[conn.Token] = conn
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
				pool.Broadcast(blockEvent)
			case req := <-newBlockListener:
				pool.Add(req.Conn)
				n.SyncRequest <- req
			case token := <-dropConnection:
				pool.Drop(token)
			}
		}
	}()

	// listen incomming
	go func() {
		for {
			if conn, err := gatewayPort.Accept(); err == nil {
				var accept socket.ValidateConnection
				if config.Firewall != nil {
					accept = config.Firewall.AcceptGateway
				} else {
					accept = socket.AcceptAllConnections
				}
				trustedConn, err := socket.PromoteConnection(conn, config.Credentials, accept)
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
				if config.Firewall != nil {
					accept = config.Firewall.AcceptBlockListener
				} else {
					accept = socket.AcceptAllConnections
				}
				trustedConn, err := socket.PromoteConnection(conn, config.Credentials, accept)
				if err != nil {
					conn.Close()
				}
				go WaitForOutgoingSyncRequest(trustedConn, newBlockListener, dropConnection, action)
			} else {
				slog.Warn("poa outgoing listener error", "error", err)
				return
			}
		}
	}()

	if config.AdminPort > 0 {
		var err error
		listenAdminPort, err = socket.Listen(fmt.Sprintf("%v:%v", config.Hostname, config.AdminPort))
		if err != nil {
			return nil, fmt.Errorf("could not listen on port %v: %v", config.BlockListenerPort, err)
		}
		go func() {
			validator := socket.ValidateSingleConnection(config.Credentials.PublicKey())
			for {
				if conn, err := listenAdminPort.Accept(); err == nil {
					trustedConn, err := socket.PromoteConnection(conn, config.Credentials, validator)
					if err != nil {
						conn.Close()
					}
					adminConnections = append(adminConnections, trustedConn)
					go AdminConnection(trustedConn, config.Firewall)
				}
			}
		}()
	}
	return n, nil
}

// WaitForProtocolActions reads proposed actions from a connection and sends them
// to the action channel. If the connection is terminated, it sends the connection
// token to the terminate channel.
func WaitForProtocolActions(conn *socket.SignedConnection, terminate chan crypto.Token, action chan []byte) {
	for {
		data, err := conn.Read()
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
func WaitForOutgoingSyncRequest(conn *socket.SignedConnection, outgoing chan SyncRequest, drop chan crypto.Token, action chan []byte) {
	lastSync := time.Now().Add(-time.Hour)
	for {
		data, err := conn.Read()
		if err != nil || len(data) < 10 || (data[0] != messages.MsgSyncRequest && data[0] != messages.MsgChecksumStatement) {
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
		}
	}
}

// AdminMsgType processes messages received from connection over the admin port.
// Messages are of the following type:
// MsgAddGateway, MsgRemoveGateway, MsgAddBlocklistener, MsgRemoveBlocklistener
// and MsgShutdown.
func AdminConnection(conn *socket.SignedConnection, firewall *Firewall) {
	for {
		msg, err := conn.Read()
		if err != nil || len(msg) < 9 {
			return
		}
		kind := AdminMsgType(msg)
		if kind >= MsgAddGateway && kind <= MsgRemoveBlocklistener {
			count, token := ParseTokenMsg(msg)
			if firewall == nil {
				conn.Send(Response(count, false))
			}
			ok := false
			switch kind {
			case MsgAddGateway:
				if firewall.AcceptGateway != nil {
					firewall.AcceptGateway.Add(token)
					ok = true
				}
			case MsgRemoveGateway:
				if firewall.AcceptGateway != nil {
					firewall.AcceptGateway.Remove(token)
					ok = true
				}
			case MsgAddBlocklistener:
				if firewall.AcceptBlockListener != nil {
					firewall.AcceptBlockListener.Add(token)
					ok = true
				}
			case MsgRemoveBlocklistener:
				if firewall.AcceptBlockListener != nil {
					firewall.AcceptBlockListener.Remove(token)
					ok = true
				}
			}
			conn.Send(Response(count, ok))
		}
	}
}
