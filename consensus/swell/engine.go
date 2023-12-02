package swell

import (
	"context"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
)

type NetworkConfiguration struct {
	NetworkHash       crypto.Hash
	BlockInterval     time.Duration
	MaxPoolSize       int
	MaxCommitteeSize  int
	BroadcastPort     int
	PoolPort          int
	ActionGatewayport int
	StateSyncPort     int
}

type ValidatorConfig struct {
	credentials crypto.PrivateKey
	walletPath  string
	swellConfig SwellNetworkConfiguration
	actions     *store.ActionStore
	relay       *relay.Node
}

const BlockInterval = time.Second

func NewGenesisNode(ctx context.Context, wallet crypto.PrivateKey, config ValidatorConfig) {
	node := &ValidatingNode{
		window:      0,
		blockchain:  chain.BlockchainFromGenesisState(wallet, config.walletPath, config.swellConfig.NetworkHash, config.swellConfig.BlockInterval),
		actions:     store.NewActionStore(1),
		credentials: config.credentials,
		committee:   SingleCommittee(config.credentials),
		swellConfig: config.swellConfig,
	}
	RunValidatorNode(ctx, node, 0)
	<-ctx.Done()
}
