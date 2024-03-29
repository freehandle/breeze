package gateway

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"fmt"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/admin"
	"github.com/freehandle/breeze/middleware/config"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
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
	clock   *ClockSync
}

func RetrieveTopology(config Configuration) (*messages.NetworkTopology, *socket.SignedConnection) {
	for _, candidate := range config.Trusted {
		addr := fmt.Sprintf("%s:%d", candidate.Addr, config.BlockRelayPort)
		provider, err := socket.Dial(config.Hostname, addr, config.Credentials, candidate.Token)
		if err == nil {
			provider.Send([]byte{messages.MsgNetworkTopologyReq})
			msg, err := provider.Read()
			if err != nil {
				continue
			}
			topology := messages.ParseNetworkTopologyMessage(msg)
			fmt.Printf("%+v\n", topology)
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
	fmt.Println("retrieving topology")
	topology, conn := RetrieveTopology(config)
	if topology == nil {
		fmt.Println("deu ruim")
		terminate <- fmt.Errorf("could not retrieve network topology")
		return terminate
	}

	proposal := make(chan *store.Propose)
	fmt.Println("launching gateway")
	LaunchGateway(ctx, config, conn, topology, proposal)

	server := Server{
		serving: make([]*socket.SignedConnection, 0),
	}

	server.clock = &ClockSync{
		SyncEpoch:     topology.Start,
		SyncEpochTime: topology.StartAt,
		BlockInterval: time.Millisecond * time.Duration(config.Breeze.BlockInterval),
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
			case req := <-administration.Interaction:
				if req.Request[0] == admin.MsgAdminReport {
					req.Response <- []byte(fmt.Sprintf("Gateway: %d clients connected", len(server.serving)))
				} else {
					req.Response <- []byte{}
				}
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

func (s *Server) WaitForActions(conn *socket.SignedConnection, proposal chan *store.Propose, terminated *sync.WaitGroup) {
	if conn == nil {
		return
	}
	bytes := util.Uint64ToBytes(s.clock.CurrentEpoch())
	conn.Send(bytes)
	for {
		data, err := conn.Read()
		fmt.Println(data)
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
		if data[0] == messages.MsgAction && len(data) > 1 {
			proposal <- &store.Propose{
				Data: data[1:],
				Conn: conn,
			}
		}
	}
	s.mu.Lock()
	for n, open := range s.serving {
		if open != nil && open.Is(conn.Token) {
			s.serving = append(s.serving[:n], s.serving[n+1:]...)
		}
	}
	s.mu.Unlock()
	conn.Shutdown()
	terminated.Done()
}
