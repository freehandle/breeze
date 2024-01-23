package blocks

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/admin"
	"github.com/freehandle/breeze/middleware/blockdb"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

type Config struct {
	Credentials crypto.PrivateKey
	DB          blockdb.DBConfig
	Swell       swell.SwellNetworkConfiguration
	Port        int
	Firewall    *socket.AcceptValidConnections
	Hostname    string
	Sources     []socket.TokenAddr
	PoolSize    int
	Protocol    uint32
}

type Server struct {
	mu          sync.Mutex
	Credentials crypto.PrivateKey
	//Source      *socket.Aggregator
	provider    *util.Chain[*blockdb.IndexedBlock]
	db          *blockdb.BlockchainHistory
	live        []*socket.SignedConnection
	subscribers []*socket.SignedConnection
	firewall    *socket.AcceptValidConnections
}

func NewServer(ctx context.Context, adm *admin.Administration, config Config) (*Server, error) {
	if config.Firewall == nil {
		return nil, fmt.Errorf("firewall config required")
	}
	ctx, cancel := context.WithCancel(ctx)
	listener := &Server{
		Credentials: config.Credentials,
		firewall:    config.Firewall,
	}
	var err error
	listener.db, err = blockdb.NewBlockchainHistory(config.DB)
	if err != nil {
		cancel()
		return nil, err
	}
	newConnListener, err := socket.Listen(fmt.Sprintf("%s:%d", config.Hostname, config.Port))
	if err != nil {
		cancel()
		return nil, err
	}

	if config.Protocol == 0 {
		NewBreezeListener(ctx, adm, config)

	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case instruction := <-adm.Interaction:
				if instruction.Request[0] == admin.MsgShutdown {
					cancel()
					instruction.Response <- []byte("shutting down")
				} else {
					instruction.Response <- []byte{}
				}
			}
		}
	}()

	go func() {
		for {
			block := listener.provider.Pop()
			if block == nil {
				return
			}
			if err := listener.db.IncorporateBlock(block); err != nil {
				slog.Warn("BlockListener: failed to incorporate block", "error", err)
			}
		}
	}()

	go func() {
		for {
			conn, err := newConnListener.Accept()
			if err != nil {
				slog.Warn("BlockListener: accept failed")
				cancel()
				return
			}
			trusted, err := socket.PromoteConnection(conn, config.Credentials, config.Firewall)
			if err != nil {
				slog.Info("BlockListener: connection rejected", "error", err)
			}
			listener.mu.Lock()
			listener.live = append(listener.live, trusted)
			listener.mu.Unlock()
		}
	}()

	return listener, nil
}

func (b *Server) Broadcast(data []byte) {
	b.mu.Lock()
	pool := make([]*socket.SignedConnection, len(b.live))
	copy(pool, b.live)
	b.mu.Unlock()
	go func() {
		for _, pool := range b.live {
			err := pool.Send(data)
			if err != nil {
				slog.Info("BlockListener: broadcast failed", "address", pool.Address, "token", pool.Token, "error", err)
				pool.Shutdown()
			}
		}
	}()
}

func BreezeListener(ctx context.Context, config Config) (*swell.StandByNode, error) {
	if config.Firewall == nil {
		return nil, fmt.Errorf("firewall config required")
	}
	ctx, cancel := context.WithCancel(ctx)
cfg := swell.ValidatorConfig{
	Credentials:    config.Credentials,
	WalletPath:     "",
	Hostname:       "localhost",
	TrustedGateway: config.Sources,
}
listener := &BreezeListenerNode{
	Credentials: config.Credentials,
	recent:      make([]*chain.CommitBlock, 0, config.keepN),
	firewall:    config.Firewall,
	keepN:       config.keepN,
}
var err error
listener.db, err = blockdb.NewBlockchainHistory(config.DB)
if err != nil {
	cancel()
	return nil, err
}
newConnListener, err := socket.Listen(fmt.Sprintf("%s:%d", config.Hostname, config.Port))
if err != nil {
	cancel()
	return nil, err
}
standBy := make(chan *swell.StandByNode)
connectedToNetwork := false
for _, source := range config.Sources {
	err = swell.FullSyncValidatorNode(ctx, cfg, source, standBy)
	if err == nil {
		connectedToNetwork = true
		break
	}
}
if !connectedToNetwork {
	cancel()
	return nil, fmt.Errorf("could not connect to network")
}
listener.Standby = <-standBy
