package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/freehandle/breeze/crypto"
)

func LoadConfig(path string) (*NodeConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open configuration file: %v\n", err)
	}
	defer file.Close()
	var config NodeConfig
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("could not parse configuration file: %v\n", err)
	}
	if err := CheckConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v\n", err)
	}
	return &config, nil
}

// NodeConfig is the configuration for a validating node
// Minimum standard configuration should provide an address, gossip, blocks and
// admin ports and a LogPath.
type NodeConfig struct {
	// Token of the Node... credentials will be provided by diffie-hellman
	Token string // `json:"token"`
	// The address of the node (IP or domain name)
	Address string // `json:"address"`
	// Port for admin connections
	AdminPort int // `json:"adminPort"`
	// WalletPath should be empty for memory based wallet store
	// OR should be a path to a valid folder with appropriate permissions
	WalletPath string // `json:"walletPath"`
	// LogPath should be empty for standard logging
	// OR should be a path to a valid folder with appropriate permissions
	LogPath string // `json:"logPath"`
	// Breeze can be left empty for standard POS configuration
	Breeze BreezeConfig // `json:"breeze"`
	// Relay can be left empty for standard Relay configuration
	Relay RelayConfig // `json:"relay"`
	// Genesis can be left empty for standard Genesis configuration
	Genesis *GenesisConfig // `json:"genesis"`
	// Trusted Nodes to connect when not actively participating in the validator
	// pool.
	TrustedNodes []Peer
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

func ParseJSON(config string) (*NodeConfig, error) {
	var node NodeConfig
	err := json.Unmarshal([]byte(config), &node)
	if err != nil {
		return nil, err
	}
	return &node, nil
}

func TokenBalanceAndDeposit(g GenesisWallet) (crypto.Token, uint64, uint64) {
	return crypto.TokenFromString(g.Token), uint64(g.Wallet), uint64(g.Deposit)
}

func isValidDir(path, scope string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("invalid %s path: %v", scope, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s path is not a directory", scope)
	}
	return nil
}

func CheckConfig(c *NodeConfig) error {
	if c == nil {
		return errors.New("no configuration specified")
	}
	if token := crypto.TokenFromString(c.Token); token.Equal(crypto.ZeroToken) {
		return errors.New("invalid token")
	}
	if _, err := net.LookupCNAME(c.Address); err != nil {
		return fmt.Errorf("could not resolver noder Address: %v", err)
	}
	if c.AdminPort < 1024 || c.AdminPort > 49151 {
		return fmt.Errorf("AdminPort must be between 1024 and 49151")
	}
	if c.WalletPath != "" {
		if err := isValidDir(c.WalletPath, "wallet"); err != nil {
			return err
		}
	}
	if err := isValidDir(c.LogPath, "log"); err != nil {
		return err
	}
	if err := CheckBreezeConfig(c.Breeze); err != nil {
		return err
	}
	if err := CheckRelayConfig(c.Relay); err != nil {
		return err
	}
	return nil
}

func CheckBreezeConfig(c BreezeConfig) error {
	if c.GossipPort < 1024 || c.GossipPort > 49151 {
		return fmt.Errorf("GossipPort must be between 1024 and 49151")
	}
	if c.BlocksPort < 1024 || c.BlocksPort > 49151 {
		return fmt.Errorf("BlocksPort must be between 1024 and 49151")
	}
	if c.BlockInterval < 500 {
		return fmt.Errorf("BlockInternval must be at least 500ms")
	}
	if time.Duration(c.ChecksumWindowBlocks*c.BlockInterval)*time.Millisecond < 10*time.Second {
		return fmt.Errorf("ChecksumWindowBlocks must be at least 10s long")
	}
	if c.Swell.CommitteeSize < 1 {
		return fmt.Errorf("Swell.CommitteeSize must be at least 1")
	}
	if c.Swell.CommitteeSize > c.ChecksumCommitteeSize {
		return fmt.Errorf("Swell.CommitteeSize must be less or equal than ChecksumCommitteeSize")
	}
	if c.Swell.ProposeTimeout < 200+c.BlockInterval {
		return fmt.Errorf("Swell.ProposeTimeout must be at least 200ms longer than BlockInternval")
	}
	if c.Swell.VoteTimeout < 200 {
		return fmt.Errorf("Swell.VoteTimeout must be at least 200ms")
	}
	if c.Swell.CommitTimeout < 200 {
		return fmt.Errorf("Swell.CommitTimeout must be at least 200ms")
	}
	if c.MaxBlockSize < 1e6 {
		return fmt.Errorf("MaxBlockSize must be at least 1MB")
	}
	if c.Permission.POA != nil && c.Permission.POS != nil {
		return fmt.Errorf("only one of POA or POS may be specified")
	}
	if c.Permission.POA != nil {
		if len(c.Permission.POA.TrustedNodes) == 0 {
			return fmt.Errorf("POA.TrustedNodes must contain at least one node")
		}
		for _, node := range c.Permission.POA.TrustedNodes {
			if crypto.TokenFromString(node).Equal(crypto.ZeroToken) {
				return fmt.Errorf("POA.TrustedNodes contains an invalid token")
			}
		}
	}
	if c.Permission.POS != nil {
		if c.Permission.POS.MinimumStake < 1e6 {
			return fmt.Errorf("POS.MinimumStake must be at least 1M")
		}
	}
	return nil
}

func CheckFirewallConfig(c FirewallConfig) error {
	if c.OpenRelay && len(c.Whitelist) > 0 {
		return errors.New("cannot have both an open relay and a whitelist")
	}
	for _, peer := range c.Whitelist {
		if crypto.TokenFromString(peer).Equal(crypto.ZeroToken) {
			return errors.New("invalid whitelist token")
		}
	}
	return nil
}

func CheckGenesisConfig(c GenesisConfig) error {
	if c.NetworkID == "" {
		return errors.New("no network ID specified")
	}
	if len(c.Wallets) == 0 {
		return errors.New("no wallets specified")
	}
	for _, wallet := range c.Wallets {
		if crypto.TokenFromString(wallet.Token).Equal(crypto.ZeroToken) {
			return fmt.Errorf("invalid token %s", wallet.Token)
		}
		if wallet.Wallet < 0 {
			return fmt.Errorf("invalid wallet %d", wallet.Wallet)
		}
		if wallet.Deposit < 0 {
			return fmt.Errorf("invalid deposit %d", wallet.Deposit)
		}
		if wallet.Deposit+wallet.Wallet == 0 {
			return fmt.Errorf("invalid wallet %d and deposit %d", wallet.Wallet, wallet.Deposit)
		}
	}
	return nil
}

func CheckRelayConfig(c RelayConfig) error {
	if c.Gateway.Port < 1024 || c.Gateway.Port > 49151 {
		return fmt.Errorf("Gateway.Port must be between 1024 and 49151")
	}
	if c.Gateway.Throughput < 1 {
		return fmt.Errorf("Gateway.Trhoughput must be at least 1")
	}
	if c.Gateway.MaxConnections < 1 {
		return fmt.Errorf("Gateway.MaxConnections must be at least 1: got %v", c.Gateway.MaxConnections)
	}
	if err := CheckFirewallConfig(c.Gateway.Firewall); err != nil {
		return fmt.Errorf("Gateway.Firewall %v", err)
	}
	if c.BlockStorage.Port < 1024 || c.BlockStorage.Port > 49151 {
		return fmt.Errorf("BlockStorage.Port must be between 1024 and 49151")
	}
	if err := isValidDir(c.BlockStorage.StoragePath, "block storage"); err != nil {
		return err
	}
	if c.BlockStorage.MaxConnections < 1 {
		return fmt.Errorf("BlockStorage.MaxConnections must be at least 1: got %v", c.BlockStorage.MaxConnections)
	}
	if err := CheckFirewallConfig(c.BlockStorage.Firewall); err != nil {
		return fmt.Errorf("BlockStorage.Firewall %v", err)
	}
	return nil
}

func CheckGenesisToken(stake crypto.Token, config *NodeConfig) error {
	if config.Genesis == nil || config.Breeze.Permission.POS == nil {
		//no relevance to check balances deposited
		return nil
	}

	deposit := 0
	for _, wallet := range config.Genesis.Wallets {
		token, _, deposit := TokenBalanceAndDeposit(wallet)
		if token.Equal(stake) {
			deposit += deposit
		}
	}
	if deposit < config.Breeze.Permission.POS.MinimumStake {
		return errors.New("node deposit is less than minimum stake, cannot start from genesis")
	}
	return nil
}
