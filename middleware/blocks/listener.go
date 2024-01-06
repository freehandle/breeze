package blocks

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/blockdb"
	"github.com/freehandle/breeze/socket"
)

type ListenerConfig struct {
	Credentials crypto.PrivateKey
	DBPath      string
}

type ListenerNode struct {
	Credentials     crypto.PrivateKey
	LastCommitEpoch uint64
	Standby         *swell.StandByNode
	db              *blockdb.BlockchainHistory
}

func NewListener(config ListenerConfig) (*ListenerNode, error) {
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
	listener.db, err = blockdb.NewBlockchainHistory(config.DBPath)
	if err != nil {
		return nil, err
	}
	standBy := make(chan *swell.StandByNode)
	err = swell.FullSyncValidatorNode(ctx, cfg, socket.TokenAddr{}, standBy)
	if err != nil {
		return nil, err
	}
	listener.Standby = <-standBy
	go func() {
		for {
			if !listener.Standby.LastEvents.Wait() {
				slog.Warn("BlockListener: last events await failed")
				cancel()
				return
			}
			recentCommit := listener.Standby.Blockchain.RecentAfter(listener.LastCommitEpoch)
			if len(recentCommit) > 0 {
				listener.IncorporateBlocks(recentCommit)
			}
		}
	}()
	return listener, nil
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
