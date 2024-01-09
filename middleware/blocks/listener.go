package blocks

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/blockdb"
	"github.com/freehandle/breeze/socket"
)

type ListenerConfig struct {
	Credentials crypto.PrivateKey
	DBPath      string
	Indexed     bool
	Port        int
	HttpInfo    bool
	HttpPort    int
	Firewall    *socket.AcceptValidConnections
	Hostname    string
}

type ListenerNode struct {
	mu              sync.Mutex
	Credentials     crypto.PrivateKey
	LastCommitEpoch uint64
	Standby         *swell.StandByNode
	db              *blockdb.BlockchainHistory
	live            []*socket.SignedConnection
	subscribers     []*socket.SignedConnection
}

func NewListener(config ListenerConfig) (*ListenerNode, error) {
	if config.Firewall == nil {
		return nil, fmt.Errorf("firewall config required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cfg := swell.ValidatorConfig{
		Credentials:    config.Credentials,
		WalletPath:     "",
		Hostname:       "localhost",
		TrustedGateway: []socket.TokenAddr{},
	}
	listener := &ListenerNode{
		Credentials: config.Credentials,
	}
	var err error
	listener.db, err = blockdb.NewBlockchainHistory(config.DBPath, true)
	if err != nil {
		return nil, err
	}
	newConnListener, err := socket.Listen(fmt.Sprintf("%s:%d", config.Hostname, config.Port))
	if err != nil {
		cancel()
		return nil, err
	}
	standBy := make(chan *swell.StandByNode)
	err = swell.FullSyncValidatorNode(ctx, cfg, socket.TokenAddr{}, standBy)
	if err != nil {
		cancel()
		return nil, err
	}
	listener.Standby = <-standBy
	go func() {
		for {
			if !listener.Standby.LastEvents.Wait() {
				slog.Warn("BlockListener: last events await failed")
				cancel()
				newConnListener.Close()
				return
			}
			recentCommit := listener.Standby.Blockchain.RecentAfter(listener.LastCommitEpoch)
			if len(recentCommit) > 0 {
				listener.IncorporateBlocks(recentCommit)
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

func (b *ListenerNode) Broadcast(data []byte) {
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

func (b *ListenerNode) IncorporateBlocks(blocks []*chain.CommitBlock) error {
	for _, block := range blocks {
		err := b.db.Incorporate(block)
		if err != nil {
			return err
		}
		b.LastCommitEpoch = block.Header.Epoch
	}
	return nil
}
