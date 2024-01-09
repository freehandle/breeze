package main

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/config"
	"github.com/freehandle/breeze/middleware/gateway"
	"github.com/freehandle/breeze/socket"
)

type Config struct {
	Token crypto.Token

	Wallet crypto.Token

	Port int

	Breeze config.BreezeConfig

	Trusted []config.Peer
}

func configToGatewayConfig(cfg Config, node, wallet crypto.PrivateKey) gateway.ConfigGateway {
	return gateway.ConfigGateway{
		Credentials:     node,
		Wallet:          wallet,
		ActionPort:      cfg.Breeze.,
		NetworkPort:     cfg.Breeze.BlocksPort,
		TrustedProvider: cfg.Breeze.TrustedProvider,
		Hostname:        cfg.Breeze.Hostname,
	}
}

type ConfigGateway struct {
	Credentials     crypto.PrivateKey
	Wallet          crypto.PrivateKey
	ActionPort      int // receive actions
	NetworkPort     int // receive checksums and send block events
	TrustedProvider socket.TokenAddr
	Hostname        string
}
