package main

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/config"
)

// NodeConfig is the configuration for a validating node
// Minimum standard configuration should provide an address, gossip, blocks and
// admin ports and a LogPath.
type NodeConfig struct {
	// Token of the Node... credentials will be provided by diffie-hellman
	Token string // `json:"token"`
	// Local Credentials path if availavle
	CredentialsPath string // `json:"credentialsPath"`
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
	Network *config.NetworkConfig // `json:"breeze"`
	// Relay can be left empty for standard Relay configuration
	Relay config.RelayConfig // `json:"relay"`
	// Genesis can be left empty for standard Genesis configuration
	Genesis *config.GenesisConfig // `json:"genesis"`
	// Trusted Nodes to connect when not actively participating in the validator
	// pool.
	TrustedNodes []config.Peer
}

func (c NodeConfig) Check() error {
	if token := crypto.TokenFromString(c.Token); token.Equal(crypto.ZeroToken) {
		return errors.New("invalid token")
	} else if c.CredentialsPath != "" {
		if data, err := os.ReadFile(c.CredentialsPath); err != nil {
			return fmt.Errorf("could not read credentials file: %v", err)
		} else if pk, err := crypto.ParsePEMPrivateKey(data); err != nil {
			return fmt.Errorf("could not parse credentials file: %v", err)
		} else {
			if !pk.PublicKey().Equal(token) {
				return fmt.Errorf("credentials file does not match token: %v instead of %v", pk.PublicKey(), token)
			}
		}
	}
	if _, err := net.LookupCNAME(c.Address); err != nil {
		return fmt.Errorf("could not resolver noder Address: %v", err)
	}
	if c.AdminPort < 1024 || c.AdminPort > 49151 {
		return fmt.Errorf("AdminPort must be between 1024 and 49151")
	}
	if c.WalletPath != "" {
		if err := config.IsValidDir(c.WalletPath, "wallet"); err != nil {
			return err
		}
	}
	if err := config.IsValidDir(c.LogPath, "log"); err != nil {
		return err
	}
	if c.Network != nil {
		if err := c.Network.Check(); err != nil {
			return err
		}
	}
	if err := c.Relay.Check(); err != nil {
		return err
	}
	return nil
}

func CheckGenesisToken(stake crypto.Token, cfg *NodeConfig) error {
	if cfg.Genesis == nil || (cfg.Network != nil && cfg.Network.Permission != nil && cfg.Network.Permission.POS == nil) {
		//no relevance to check balances deposited
		return nil
	}

	var minimumStake int
	if cfg.Network == nil || cfg.Network.Permission == nil {
		minimumStake = config.StandardPoSConfig.POS.MinimumStake
	} else {
		minimumStake = cfg.Network.Permission.POS.MinimumStake
	}

	deposit := 0
	for _, wallet := range cfg.Genesis.Wallets {
		token, _, deposit := TokenBalanceAndDeposit(wallet)
		if token.Equal(stake) {
			deposit += deposit
		}
	}
	if deposit < minimumStake {
		return errors.New("node deposit is less than minimum stake, cannot start from genesis")
	}
	return nil
}

func TokenBalanceAndDeposit(g config.GenesisWallet) (crypto.Token, uint64, uint64) {
	return crypto.TokenFromString(g.Token), uint64(g.Wallet), uint64(g.Deposit)
}
