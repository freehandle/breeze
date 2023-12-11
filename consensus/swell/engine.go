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

// SwellNetrworkConfiguration defines the parameters for the unerlying crypto
// network running the swell protocol.
type SwellNetworkConfiguration struct {
	NetworkHash      crypto.Hash   // there should be a unique hash for each network
	MaxPoolSize      int           // max number of validator in each hash consensus pool
	MaxCommitteeSize int           // max number of validators in the checksum window
	BlockInterval    time.Duration // time duration for each block
	ChecksumWindow   int           // number of blocks in the checksum window
	Permission       Permission    // permission rules to be a validator in the network
}

// Permission is an interface that defines the rules for a validator
type Permission interface {
	Punish(duplicates *bft.Duplicate, weights map[crypto.Token]int) map[crypto.Token]uint64
	DeterminePool(chain *chain.Blockchain, candidates []crypto.Token) map[crypto.Token]int
}

type BlockConsensusConfirmation struct {
	Epoch  uint64
	Status bool
}

// ValidatorConfig defines the configuration for a validator node.
type ValidatorConfig struct {
	credentials crypto.PrivateKey
	walletPath  string
	swellConfig SwellNetworkConfiguration
	actions     *store.ActionStore
	relay       *relay.Node
	hostname    string
}

const BlockInterval = time.Second

// NewGenesisNode creates a new blockchain from genesis state (associated to the
// given wallet), and starts a validating node interacting with the given relay
// network.
func NewGenesisNode(ctx context.Context, wallet crypto.PrivateKey, config ValidatorConfig) {
	token := config.credentials.PublicKey()
	node := &SwellNode{
		blockchain:  chain.BlockchainFromGenesisState(wallet, config.walletPath, config.swellConfig.NetworkHash, config.swellConfig.BlockInterval, config.swellConfig.ChecksumWindow),
		actions:     store.NewActionStore(ctx, 1),
		credentials: config.credentials,
		validators: &ChecksumWindowValidators{
			order:   []crypto.Token{token},
			weights: map[crypto.Token]int{token: 1},
		},
		config:   config.swellConfig,
		relay:    config.relay,
		hostname: config.hostname,
	}
	RunActionsGateway(ctx, config.relay.ActionGateway, node.actions)
	node.RunValidatingNode(ctx, SingleCommittee(config.credentials, config.hostname), 0)
	<-ctx.Done()
}

// RunActionsGateway keep track of the actions gateway channel and populates the
// node action store with information gathered from there.
func RunActionsGateway(ctx context.Context, gateway chan []byte, store *store.ActionStore) {
	done := ctx.Done()
	go func() {
		for {
			select {
			case action := <-gateway:
				if store.Live {
					store.Push <- action
				} else {
					return
				}
			case <-done:
				return
			}
		}
	}()
}
