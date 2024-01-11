package gateway

import (
	"context"
	"log/slog"
	"sync"

	"fmt"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

const (
	ActionMsg byte = iota + 1
	ActionResponse
	Bye
)

type Configuration struct {
	Credentials     crypto.PrivateKey
	Hostname        string
	Port            int
	Firewall        *socket.AcceptValidConnections
	Trusted         []socket.TokenAddr
	ActionRelayPort int
	BlockRelayPort  int
}

type Server struct {
	mu      sync.Mutex
	serving []*socket.SignedConnection
}

func RetrieveTopology(config Configuration) ([]crypto.Token, []socket.TokenAddr) {
	for _, candidate := range config.Trusted {
		addr := fmt.Sprintf("%s:%d", candidate.Addr, config.BlockRelayPort)
		provider, err := socket.Dial(config.Hostname, addr, config.Credentials, candidate.Token)
		if err == nil {
			provider.Send([]byte{messages.MsgNetworkTopologyReq})
			msg, err := provider.Read()
			if err != nil {
				continue
			}
			order, validators := swell.ParseCommitee(msg)
			if len(order) > 0 && len(validators) > 0 {
				return order, validators
			}
		}
	}
	return nil, nil
}

func NewGateway(ctx context.Context, config Configuration) chan error {
	terminate := make(chan error, 2)
	listener, err := socket.Listen(fmt.Sprintf("%s:%d", config.Hostname, config.Port))
	if err != nil {
		terminate <- err
		return terminate
	}

	order, validators := RetrieveTopology(config)
	provider.Send([]byte{messages.MsgNetworkTopologyReq})

	server := Server{
		serving: make([]*socket.SignedConnection, 0),
	}

	proposal := make(chan Propose)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				terminate <- err
				listener.Close()
				return
			}
			trusted, err := socket.PromoteConnection(conn, config.Credentials, config.Firewall)
			if err != nil {
				slog.Info("could not promote connection", "error", err)
			}
			server.mu.Lock()
			server.serving = append(server.serving, trusted)
			server.mu.Unlock()
			go server.WaitForActions(trusted, proposal)
		}
	}()
	return terminate
}

func (s *Server) WaitForActions(conn *socket.SignedConnection, proposal chan Propose) {
	for {
		data, err := conn.Read()
		if err != nil {
			break
		}
		if len(data) == 0 {
			continue
		}
		if data[0] == Bye {
			slog.Info("connection terminated by client", "token", conn.Token)
			break
		}
		if data[1] == ActionMsg && len(data) > 1 {
			proposal <- Propose{
				data: data[1:],
				conn: conn,
			}
		}
	}
	s.mu.Lock()
	for n, open := range s.serving {
		if open.Is(conn.Token) {
			s.serving = append(s.serving[:n], s.serving[n+1:]...)
		}
	}
	s.mu.Unlock()
	conn.Shutdown()
}
