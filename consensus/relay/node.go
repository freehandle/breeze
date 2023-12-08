package relay

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

type Shutdowner interface {
	Shutdown()
}

type Config struct {
	GatewayPort       int
	BlockListenerPort int
	AdminPort         int
	Firewall          *Firewall
	Credentials       crypto.PrivateKey
	Hostname          string
}

func NewFireWall(authorizedGateway []crypto.Token, autorizedBlockListener []crypto.Token) *Firewall {
	return &Firewall{
		AcceptGateway:       socket.NewValidConnections(authorizedGateway),
		AcceptBlockListener: socket.NewValidConnections(autorizedBlockListener),
	}

}

type Firewall struct {
	AcceptGateway       *socket.AcceptValidConnections
	AcceptBlockListener *socket.AcceptValidConnections
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

func Run(ctx context.Context, config Config) (*Node, error) {
	n := &Node{
		ActionGateway: make(chan []byte),
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
				go WaitForOutgoingSyncRequest(trustedConn, newBlockListener)
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
		if err != nil || len(data) < 2 || data[0] != chain.MsgActionSubmit {
			if err != nil {
				slog.Info("poa WaitForProtocolActions: connection terminated", "connection", err)
			} else {
				slog.Info("poa WaitForProtocolActions: invalid action", "connection", conn.Token, "data", data)
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
	if err != nil || len(data) != 10 || data[0] != chain.MsgSyncRequest {
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