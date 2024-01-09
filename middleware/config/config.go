package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type Configurable interface {
	Check() error
}

func LoadConfig[T Configurable](path string) (*T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open configuration file: %v", err)
	}
	defer file.Close()
	var config T
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("could not parse configuration file: %v", err)
	}
	if err := config.Check(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}
	return &config, nil
}

func ParseJSON[T any](config string) (*T, error) {
	var node T
	err := json.Unmarshal([]byte(config), &node)
	if err != nil {
		return nil, err
	}
	return &node, nil
}

// SwellConfig is the configuration for parameters defining the Swell BFT
// protocol
type SwellConfig struct {
	// CommitteeSize is the number of nodes to use for the swell consensus
	// committe
	CommitteeSize int // `json:"committeeSize"`
	// ProposeTimeout is the number of milliseconds to wait for a proposal for
	// the hash of the block. It counts starting from the start of block minting
	// period. Timeout must be set taking into consideration the block interval
	// and the expected latency on the network.
	ProposeTimeout int // `json:"proposeTimeout"`
	// VoteTimeout is the number of milliseconds to wait in vote state.  Must be
	// set taking into consideration the latency of the network.
	VoteTimeout int // `json:"voteTimeout"`
	// VoteTimeout is the number of milliseconds to wait in commit state.  Must
	// be set taking into consideration the latency of the network and should
	// tipically be the same as the vote timeout.
	CommitTimeout int // `json:"commitTimeout"`
}

// PermissionConfig is the configuration for the permissioning protocol.
// At most one of the two fields should be set. If none is set, the netowrk will
// operate under permissionless consensus. This should only be deployed on
// secure private networks.
type PermissionConfig struct {
	POA *POAConfig // `json:"poa"`
	POS *POSConfig // `json:"pos"`
}

// POAConfig is the configuration for the proof-of-authority permissioning
// protocol. TrustedNodes can be set only with the initial authority token and
// be modified by the admin console later.
type POAConfig struct {
	TrustedNodes []string // `json:"trustedNodes"`
}

// POSConfig is the configuration for the proof-of-stake permissioning protocol.
type POSConfig struct {
	// MinimumStake is the minimum amount of tokens deposited for a node to be
	// eligible for the committee.
	MinimumStake int // `json:"minimumStake"`
}

// BreezeConfig is the configuration for parameters defining the Breeze protocol
type BreezeConfig struct {
	// Port for gossip network connections
	GossipPort int // `json:"gossipPort"`
	// Port for block broadcast connections
	BlocksPort int // `json:"blocksPort"`
	// Permission: POS or POA configs
	Permission PermissionConfig // `json:"permission"`
	// BlockInterval is the number of milliseconds between blocks. At minimum
	// this should be 500ms
	BlockInterval int // `json:"blockInterval"`
	// ChecksumWindowBlocks is the number of blocks to use for the checksum
	// window. Checksum window must be at least 10 seconds worth of blocks.
	ChecksumWindowBlocks int // `json:"checksumWindowBlocks"`
	// ChecksumCommitteeSize is the number of nodes to use for the checksum
	// committee. Checksum should be a small multiple of the swell committee
	// size.
	ChecksumCommitteeSize int // `json:"checksumCommitteeSize"`
	// MaxBlockSize is the maximum size of a block in bytes that mitigates the
	// risk of a DDOS attack. It must be at least 1MB.
	MaxBlockSize int // `json:"maxBlockSize"`
	// Configurations for the parameters defining the Swell protocol
	Swell SwellConfig // `json:"swell"`
}

type GenesisWallet struct {
	Token   string // `json:"token"`
	Wallet  int    // `json:"amount"`
	Deposit int    // `json:"deposited"`
}

type Peer struct {
	Address string // `json:"address"`
	Token   string // `json:"token"`
}

// GenesisConfig is the configuration for the genesis block
type GenesisConfig struct {
	Wallets   []GenesisWallet // `json:"wallets"`
	NetworkID string          // `json:"networkID"`
}

type FirewallConfig struct {
	OpenRelay bool // `json:"openRelay"`
	// Whitelist is a list of addresses that are allowed to connect to the node
	Whitelist []string // `json:"whitelist"`
}

type RelayConfig struct {
	Gateway      GatewayConfig      // `json:"gateway"`
	BlockStorage BlockStorageConfig // `json:"blockStorage"`
}

type GatewayConfig struct {
	Port int // `json:"port"`
	// Number of actions per BlockInterval
	Throughput int // `json:"throughput"`
	// Max number of simultaneous connections
	MaxConnections int // `json:"maxConnections"`
	// Firewall rules
	Firewall FirewallConfig // `json:"firewall"`
}

type BlockStorageConfig struct {
	Port int // `json:"port"`
	// Directory to storage block history
	StoragePath string // `json:"storagePath"`
	// Indicate if an index of tokens should be mantained
	IndexWallets bool // `json:"indexWallets"
	// Max number of simultaneous connections
	MaxConnections int // `json:"maxConnections"`
	// Firewall rules
	Firewall FirewallConfig
}

var StandardSwellConfig = SwellConfig{
	CommitteeSize:  10,
	ProposeTimeout: 1500,
	VoteTimeout:    1500,
	CommitTimeout:  1500,
}

var StandardPOSBreezeConfig = BreezeConfig{
	GossipPort: 5401,
	BlocksPort: 5402,
	Permission: PermissionConfig{
		POS: &POSConfig{
			MinimumStake: 1e6,
		},
	},
	BlockInterval:         1000,
	ChecksumWindowBlocks:  900,
	ChecksumCommitteeSize: 100,
	MaxBlockSize:          1e9,
	Swell:                 StandardSwellConfig,
}

func StandardPOABreezeConfig(authority crypto.Token) BreezeConfig {
	return BreezeConfig{
		GossipPort: 5401,
		BlocksPort: 5402,
		Permission: PermissionConfig{
			POA: &POAConfig{
				TrustedNodes: []string{authority.String()},
			},
		},
		BlockInterval:         1000,
		ChecksumWindowBlocks:  900,
		ChecksumCommitteeSize: 100,
		MaxBlockSize:          1e9,
		Swell:                 StandardSwellConfig,
	}
}

func FirewallToValidConnections(f FirewallConfig) *socket.AcceptValidConnections {
	tokens := make([]crypto.Token, 0)
	for _, tokenStr := range f.Whitelist {
		token := crypto.TokenFromString(tokenStr)
		if token != crypto.ZeroToken {
			tokens = append(tokens, token)
		}
	}
	return socket.NewValidConnections(tokens, f.OpenRelay)
}

func PeersToTokenAddr(peers []Peer) []socket.TokenAddr {
	tk := make([]socket.TokenAddr, 0)
	for _, peer := range peers {
		token := crypto.TokenFromString(peer.Token)
		if token != crypto.ZeroToken {
			tk = append(tk, socket.TokenAddr{
				Token: token,
				Addr:  peer.Address,
			})
		}
	}
	return tk
}
