package swell

import (
	"log/slog"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type NetworkConfiguration struct {
	BlockInterval     time.Duration
	MaxPoolSize       int
	MaxCommitteeSize  int
	BroadcastPort     int
	PoolPort          int
	ActionGatewayport int
	StateSyncPort     int
}

const BlockInterval = time.Second

type ValidatorConfig struct {
	Chain *chain.Blockchain
}

func NewGenesisNode(wallet, node crypto.PrivateKey, walletpath string) chan error {
	terminate := make(chan error, 2)

	token := node.PublicKey()

	validator := Node{
		checkpoint:   0,
		blockchain:   chain.BlockchainFromGenesisState(wallet, walletpath),
		actions:      store.NewActionStore(1),
		credentials:  node,
		channel:      make([]*socket.ChannelConnection, 0),
		broadcasting: nil,
		validators:   make([]socket.CommitteeMember, 0),
		weights:      map[crypto.Token]int{token: 1},
		order:        []crypto.Token{token},
	}

	go func() {
		tick := time.NewTicker(BlockInterval)
		epoch := uint64(0)
		consensusConfirmation := make(chan BlockConsensusConfirmation)
		for {
			select {
			case <-tick.C:
				epoch += 1
				validator.RunEpoch(epoch, nil)
			case confirmation := <-consensusConfirmation:
				if confirmation.Status {
					if validator.checkpoint < confirmation.Epoch {
						validator.checkpoint = confirmation.Epoch
					}
				} else {
					slog.Warn("validator consensus failed for epoch", "epoch", confirmation.Epoch)
				}
			}
		}
	}()

	return terminate
}

func NewValidatorNode(config *ValidatorConfig, joinEpoch uint64) {

}
