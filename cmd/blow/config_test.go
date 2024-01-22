package main

import (
	"testing"

	"github.com/freehandle/breeze/middleware/config"
)

var configTest string = `
{
	"address": "192.168.0.1",
	"adminPort": 5403,
	"walletPath": "/var/breeze/wallet",
	"logPath": "/var/breeze/log",
	"breeze": {
		"gossipPort": 5401,
		"blocksPort": 5402,
		"permission": {
			"pos" : {
				"minimumStake": 1000000
			}
		}
	},
	"relay": {
		"gateway": {
			"port": 5404,
			"throughput": 5000,
			"firewall": {
				"openRelay": false,
				"whitelist": [
					{
						"address": "192.168.0.2",
						"token": "7eb7aa3582b216bba42d45e91e0a560508478f5b55228439b42733945fd5c2f5"
					}
				]
			}
		},
		"blockStorage": {
			"port": 5405,
			"storagePath": "/var/breeze/blocks",
			"indexWallets": true,
			"firewall": {
				"openRelay": true
			}
		}
	},
	"genesis": {
		"wallets": [
			{
				"token": "7eb7aa3582b216bba42d45e91e0a560508478f5b55228439b42733945fd5c2f5",
				"wallet": 999000000,
				"deposit": 1000000
			}
		],
		"networkID": "testnet"
	}
}
`

func TestConfigParse(t *testing.T) {
	token := "7eb7aa3582b216bba42d45e91e0a560508478f5b55228439b42733945fd5c2f5"
	c, err := config.ParseJSON[NodeConfig](configTest)
	if err != nil {
		t.Error(err)
	}
	if c.Address != "192.168.0.1" {
		t.Error("Address field not parsed correctly")
	}
	if c.AdminPort != 5403 {
		t.Error("AdminPort field not parsed correctly")
	}
	if c.WalletPath != "/var/breeze/wallet" {
		t.Error("WalletPath field not parsed correctly")
	}
	if c.LogPath != "/var/breeze/log" {
		t.Error("LogPath field not parsed correctly")
	}
	if c.Network.Breeze.GossipPort != 5401 {
		t.Error("GossipPort field not parsed correctly")
	}
	if c.Network.Breeze.BlocksPort != 5402 {
		t.Error("BlocksPort field not parsed correctly")
	}
	if c.Network.Permission.POS.MinimumStake != 1000000 {
		t.Error("MinimumStake field not parsed correctly")
	}
	if c.Network.Permission.POA != nil {
		t.Error("POA field not parsed correctly")
	}
	if c.Relay.Gateway.Port != 5404 {
		t.Error("Gateway.Port field not parsed correctly")
	}
	if c.Relay.Gateway.Throughput != 5000 {
		t.Error("Gateway.Throughput field not parsed correctly")
	}
	if c.Relay.Gateway.Firewall.Open {
		t.Error("Gateway.Firewall.OpenRelay field not parsed correctly")
	}
	if len(c.Relay.Gateway.Firewall.TokenList) != 1 {
		t.Error("Gateway.Firewall.Whitelist field not parsed correctly")
	}
	if c.Relay.Gateway.Firewall.TokenList[0] != token {
		t.Error("Gateway.Firewall.Whitelist[0].Token field not parsed correctly")
	}
	if c.Relay.Blocks.Port != 5405 {
		t.Error("BlockStorage.Port filed not parsed correctly")
	}
	if c.Relay.Blocks.StoragePath != "/var/breeze/blocks" {
		t.Error("BlockStorage.StoragePath filed not parsed correctly")
	}
	if !c.Relay.Blocks.IndexWallets {
		t.Error("BlockStorage.IndexWallets filed not parsed correctly")
	}
	if !c.Relay.Blocks.Firewall.Open {
		t.Error("BlockStorage.Firewall.OpenRelay filed not parsed correctly")
	}
	if len(c.Genesis.Wallets) != 1 {
		t.Error("Genesis.Wallets field not parsed correctly")
	}
	if c.Genesis.Wallets[0].Token != token {
		t.Error("Genesis.Wallets field not parsed correctly")
	}
	if c.Genesis.Wallets[0].Wallet != 999000000 {
		t.Error("Genesis.Wallets field not parsed correctly")
	}
	if c.Genesis.Wallets[0].Deposit != 1000000 {
		t.Error("Genesis.Wallets field not parsed correctly")
	}
	if c.Genesis.NetworkID != "testnet" {
		t.Error("Genesis.NetworkID field not parsed correctly")
	}
}
