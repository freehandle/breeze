package blocks

import (
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/blockdb"
	"github.com/freehandle/breeze/socket"
)

type ListenerConfig struct {
	Credentials crypto.PrivateKey
	DB          blockdb.DBConfig
	Swell       swell.SwellNetworkConfiguration
	Port        int
	Firewall    *socket.AcceptValidConnections
	Hostname    string
	Sources     []socket.TokenAddr
	keepN       int
}

/*
type BreezeListenerNode struct {
	mu              sync.Mutex
	Credentials     crypto.PrivateKey
	LastCommitEpoch uint64
	Standby         *swell.StandByNode
	db              *blockdb.BlockchainHistory
	live            []*socket.SignedConnection
	subscribers     []*socket.SignedConnection
	recent          []*chain.CommitBlock
	firewall        *socket.AcceptValidConnections
	keepN           int
}

func NewBreezeListener(ctx context.Context, adm *admin.Administration, config ListenerConfig) (*BreezeListenerNode, error) {
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

func (b *BreezeListenerNode) Broadcast(data []byte) {
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

func (b *BreezeListenerNode) IncorporateBlocks(blocks []*chain.CommitBlock) error {
	for _, block := range blocks {
		err := b.db.Incorporate(block)
		if err != nil {
			return err
		}
		b.LastCommitEpoch = block.Header.Epoch
		if len(b.recent) == b.keepN {
			b.recent = append(b.recent[1:], block)
		} else {
			b.recent = append(b.recent, block)
		}
	}
	return nil
}

func (b *BreezeListenerNode) Recent() []*chain.CommitBlock {
	b.mu.Lock()
	blocks := make([]*chain.CommitBlock, len(b.recent))
	copy(blocks, b.recent)
	b.mu.Unlock()
	return blocks
}
*/
