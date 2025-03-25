package swell

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/admin"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
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
	Credentials crypto.PrivateKey
	WalletPath  string
	SwellConfig SwellNetworkConfiguration
	//Actions        *store.ActionStore
	Relay          *relay.Node
	Admin          *admin.Administration
	Hostname       string
	TrustedGateway []socket.TokenAddr
}

// const BlockInterval = time.Second

// NewGenesisNode creates a new blockchain from genesis state (associated to the
// given wallet), and starts a validating node interacting with the given relay
// network.
func NewGenesisNode(ctx context.Context, wallet crypto.PrivateKey, config ValidatorConfig) *SwellNode {
	token := config.Credentials.PublicKey()
	node := &SwellNode{
		blockchain: chain.BlockchainFromGenesisState(wallet, config.WalletPath, config.SwellConfig.NetworkHash, config.SwellConfig.BlockInterval, config.SwellConfig.ChecksumWindow),
		actions:    store.NewActionStore(ctx, 1, config.Relay.ActionGateway),
		//actions:     store.NewActionVaultNoReply(ctx, 1, config.Relay.ActionGateway),
		credentials: config.Credentials,
		validators: &ChecksumWindowValidators{
			order:   []crypto.Token{token},
			weights: map[crypto.Token]int{token: 1},
		},
		config:   config.SwellConfig,
		relay:    config.Relay,
		admin:    config.Admin,
		hostname: config.Hostname,
	}
	//RunActionsGateway(ctx, config.Relay.ActionGateway, node.actions)
	go node.ServeAdmin(ctx)
	window := Window{
		ctx:       ctx,
		Start:     1,
		End:       uint64(config.SwellConfig.ChecksumWindow),
		Node:      node,
		Committee: SingleCommittee(config.Credentials, config.Hostname),
	}
	RunValidator(&window)
	//node.RunValidatingNode(ctx, SingleCommittee(config.Credentials, config.Hostname), 0)
	return node
}

// RunActionsGateway keep track of the actions gateway channel and populates the
// node action store with information gathered from there.
/*func RunActionsGateway(ctx context.Context, gateway chan []byte, store *store.ActionVault) {
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
*/

func ConnectToTrustedGateway(ctx context.Context, config ValidatorConfig) error {
	for _, gateway := range config.TrustedGateway {
		conn, err := socket.Dial(config.Hostname, gateway.Addr, config.Credentials, gateway.Token)
		if err != nil {
			return err
		}
		bytes := []byte{messages.MsgSyncRequest}
		util.PutUint64(0, &bytes)
		util.PutBool(true, &bytes)
		conn.Send(bytes)
	}
	return nil
}

// ChecksumWindowValidators defines the order of tokens for the current checksum
// window and the weight of each token.
type ChecksumWindowValidators struct {
	order   []crypto.Token
	weights map[crypto.Token]int
}

type CandidateStatus struct {
	Dressed bool
	Naked   bool
}

// SwellNode is the basic structure to run a swell node either as a validator,
// as a candidate validator or simplu as an iddle observer.
// the internal clock of the node is used to determine the current epoch of the
// breeze network.
// It can be optionally linked to a relay node to communicate with the ouside
// world. Relay network is used to gather proposed actions through the actions
// gateway and to send block events to other interested parties. Honest validator
// must keep the relay network open with and adequate firewall rule. Swell
// protocol does not dictate rules for the realy network nonetheless.
type SwellNode struct {
	validators  *ChecksumWindowValidators // valdiators for the current cheksum window
	credentials crypto.PrivateKey         // credentials for the node
	blockchain  *chain.Blockchain         // node's version of breeze blockchain
	actions     *store.ActionStore        // actions received through the actions gateway
	//actions  *store.ActionVault
	admin    *admin.Administration     // administration interface
	config   SwellNetworkConfiguration // parameters of the underlying network
	active   chan chan error
	relay    *relay.Node // (optional) relay network
	hostname string      // "localhost" or empty for internet, anything else for testing
	cancel   context.CancelFunc
}

func (s *SwellNode) AdminReport() string {
	status := ""
	if s.relay != nil {
		status = fmt.Sprintf("%vRelay Report\n===========\n%v", status, s.relay.Status())
	}
	return status
}

func (s *SwellNode) ServeAdmin(ctx context.Context) {
	for {
		select {
		case req := <-s.admin.Interaction:
			if req.Request[0] == admin.MsgAdminReport {
				req.Response <- []byte(s.AdminReport())
			} else {

			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *SwellNode) TimeStampBlock(epoch uint64) time.Time {
	delta := time.Duration(epoch-s.blockchain.Clock.Epoch) * s.config.BlockInterval
	slog.Info("TimeStampBlock", "epoch", epoch, "delta", delta)
	return s.blockchain.Clock.TimeStamp.Add(delta)

}

func (s *SwellNode) Timer(epoch uint64) *time.Timer {
	delta := time.Until(s.blockchain.TimestampBlock(epoch))
	// minimum timer is set as 100 miliseconds... in order to prevent
	// the node from being too busy doing many things ate once.
	if delta < s.config.BlockInterval/10 {
		delta = s.config.BlockInterval / 10
	}
	return time.NewTimer(delta)
}

func (s *SwellNode) PurgeActions(actions *chain.ActionArray) {
	for n := 0; n < actions.Len(); n++ {
		hash := crypto.Hasher(actions.Get(n))
		s.actions.Exlude(hash)
	}
}

// Permanently exlude action from the underluing action store.
func (s *SwellNode) PurgeAction(hash crypto.Hash) {
	s.actions.Exlude(hash)
}
