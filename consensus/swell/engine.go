package swell

import (
	"context"
	"time"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
)

type SwellNetworkConfiguration struct {
	NetworkHash      crypto.Hash
	MaxPoolSize      int
	MaxCommitteeSize int
	BlockInterval    time.Duration
	ChecksumWindow   int
	Permission       Permission
}

const (
	MaxPoolSize      = 10
	MaxCommitteeSize = 100
)

type Permission interface {
	Punish(duplicates *bft.Duplicate, weights map[crypto.Token]int) map[crypto.Token]uint64
	DeterminePool(chain *chain.Blockchain, candidates []crypto.Token) Validators
}

type BlockConsensusConfirmation struct {
	Epoch  uint64
	Status bool
}

type ValidatingNode struct {
	//genesisTime  time.Time
	window      uint64            // epoch starting current checksum window
	blockchain  *chain.Blockchain // nodes of distinct windows can have this pointer concurrently
	actions     *store.ActionStore
	credentials crypto.PrivateKey
	committee   *ChecksumWindowValidatorPool
	swellConfig SwellNetworkConfiguration
	relay       *relay.Node
}

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
	token := config.credentials.PublicKey()
	node := &SwellNode{
		blockchain:  chain.BlockchainFromGenesisState(wallet, config.walletPath, config.swellConfig.NetworkHash, config.swellConfig.BlockInterval),
		actions:     store.NewActionStore(1),
		credentials: config.credentials,
		validators: &ChecksumWindowValidators{
			order:   []crypto.Token{token},
			weights: map[crypto.Token]int{token: 1},
		},
		config: config.swellConfig,
	}
	node.RunValidatingNode(ctx, SingleCommittee(config.credentials), 0)
	<-ctx.Done()
}
