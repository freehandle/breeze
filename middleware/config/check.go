package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/freehandle/breeze/crypto"
)

func (c NetworkConfig) Check() error {
	if c.Breeze != nil {
		if err := c.Breeze.Check(); err != nil {
			return fmt.Errorf("Breeze %v", err)
		}
	}
	if c.Permission != nil {
		if err := c.Permission.Check(); err != nil {
			return fmt.Errorf("Permission %v", err)
		}
	}
	return nil
}

func (c PermissionConfig) Check() error {
	if c.POA != nil && c.POS != nil {
		return fmt.Errorf("only one of POA or POS may be specified")
	}
	if c.POA != nil {
		if len(c.POA.TrustedNodes) == 0 {
			return fmt.Errorf("POA.TrustedNodes must contain at least one node")
		}
		for _, node := range c.POA.TrustedNodes {
			if crypto.TokenFromString(node).Equal(crypto.ZeroToken) {
				return fmt.Errorf("POA.TrustedNodes contains an invalid token")
			}
		}
	}
	if c.POS != nil {
		if c.POS.MinimumStake < 1e6 {
			return fmt.Errorf("POS.MinimumStake must be at least 1M")
		}
	}
	return nil
}

func (c BreezeConfig) Check() error {
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
	return nil

}

func (c FirewallConfig) Check() error {
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

func (c GenesisConfig) Check() error {
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

func (c RelayConfig) Check() error {
	if c.Gateway.Port < 1024 || c.Gateway.Port > 49151 {
		return fmt.Errorf("Gateway.Port must be between 1024 and 49151")
	}
	if c.Gateway.Throughput < 1 {
		return fmt.Errorf("Gateway.Trhoughput must be at least 1")
	}
	if c.Gateway.MaxConnections < 1 {
		return fmt.Errorf("Gateway.MaxConnections must be at least 1: got %v", c.Gateway.MaxConnections)
	}
	if err := c.Gateway.Firewall.Check(); err != nil {
		return fmt.Errorf("Gateway.Firewall %v", err)
	}
	if c.BlockStorage.Port < 1024 || c.BlockStorage.Port > 49151 {
		return fmt.Errorf("BlockStorage.Port must be between 1024 and 49151")
	}
	if err := IsValidDir(c.BlockStorage.StoragePath, "block storage"); err != nil {
		return err
	}
	if c.BlockStorage.MaxConnections < 1 {
		return fmt.Errorf("BlockStorage.MaxConnections must be at least 1: got %v", c.BlockStorage.MaxConnections)
	}
	if err := c.BlockStorage.Firewall.Check(); err != nil {
		return fmt.Errorf("BlockStorage.Firewall %v", err)
	}
	return nil
}

func IsValidDir(path, scope string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("invalid %s path: %v", scope, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s path is not a directory", scope)
	}
	return nil
}
