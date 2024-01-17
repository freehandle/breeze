package gateway

import (
	"context"
	"log/slog"
	"sync"

	"fmt"

	"github.com/freehandle/breeze/consensus/admin"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/config"
	"github.com/freehandle/breeze/socket"
)

const (
	ActionMsg byte = iota + 1
	ActionResponse
	Bye
)

type Configuration struct {
	Credentials     crypto.PrivateKey
	Wallet          crypto.PrivateKey
	Hostname        string
	Port            int
	Firewall        *socket.AcceptValidConnections
	Trusted         []socket.TokenAddr
	ActionRelayPort int
	BlockRelayPort  int
	Breeze          config.BreezeConfig
}

type Server struct {
	mu      sync.Mutex
	serving []*socket.SignedConnection
}

func RetrieveTopology(config Configuration) (*messages.NetworkTopology, *socket.SignedConnection) {
	for _, candidate := range config.Trusted {
		addr := fmt.Sprintf("%s:%d", candidate.Addr, config.BlockRelayPort)
		provider, err := socket.Dial(config.Hostname, addr, config.Credentials, candidate.Token)
		if err == nil {
			fmt.Println("sent")
			provider.Send([]byte{messages.MsgNetworkTopologyReq})
			msg, err := provider.Read()
			if err != nil {
				continue
			}
			topology := messages.ParseNetworkTopologyMessage(msg)
			if topology != nil {
				return topology, provider
			}
		}
	}
	return nil, nil
}

func NewServer(ctx context.Context, config Configuration, administration *admin.Administration) chan error {
	terminate := make(chan error, 2)
	listener, err := socket.Listen(fmt.Sprintf("%s:%d", config.Hostname, config.Port))
	if err != nil {
		terminate <- err
		return terminate
	}
	topology, conn := RetrieveTopology(config)
	if topology == nil {
		terminate <- fmt.Errorf("could not retrieve network topology")
		return terminate
	}

	proposal := make(chan *Propose)
	LaunchGateway(ctx, config, conn, topology, proposal)

	server := Server{
		serving: make([]*socket.SignedConnection, 0),
	}

	liveClients := &sync.WaitGroup{}

	go func() {
		done := ctx.Done()
		for {
			select {
			case <-done:
				listener.Close()
				server.mu.Lock()
				for _, conn := range server.serving {
					conn.Shutdown()
				}
				server.mu.Unlock()
				liveClients.Wait()
				close(proposal)
				return
			case firewall := <-administration.FirewallAction:
				if firewall.Scope == admin.GrantGateway {
					config.Firewall.Add(firewall.Token)
				} else if firewall.Scope == admin.RevokeGateway {
					config.Firewall.Remove(firewall.Token)
				}
			case status := <-administration.Status:
				status <- fmt.Sprintf("Gateway: %d clients connected", len(server.serving))
			}
		}

	}()

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
			liveClients.Add(1)
			go server.WaitForActions(trusted, proposal, liveClients)
		}
	}()
	return terminate
}

func (s *Server) WaitForActions(conn *socket.SignedConnection, proposal chan *Propose, terminated *sync.WaitGroup) {
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
		if data[0] == ActionMsg && len(data) > 1 {
			proposal <- &Propose{
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
	terminated.Done()
}
