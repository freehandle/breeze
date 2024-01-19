package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/freehandle/breeze/consensus/permission"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/admin"
	"github.com/freehandle/breeze/middleware/config"
	"github.com/freehandle/breeze/socket"
)

const usage = "usage: blow <path-to-json-config-file> [genesis|sync address token]"

func main() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	if len(os.Args) < 3 {
		fmt.Println(usage)
		os.Exit(1)
	}
	cfg, err := config.LoadConfig[NodeConfig](os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if cfg.Network == nil {
		cfg.Network = config.StandardBreezeNetworkConfig
	}
	if cfg.Network.Breeze == nil {
		cfg.Network.Breeze = config.StandardBreezeConfig
	}
	if cfg.Network.Permission == nil {
		cfg.Network.Permission = config.StandardPoSConfig
	}

	log := filepath.Join(cfg.LogPath, fmt.Sprintf("%v.log", cfg.Token[0:16]))

	logFile, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("could not open log file: %v\n", err)
		os.Exit(1)
	}
	var programLevel = new(slog.LevelVar) // Info by default
	programLevel.Set(slog.LevelDebug)
	logger := slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: programLevel})
	slog.SetDefault(slog.New(logger))

	swellConfig := SwellConfigFromConfig(cfg)
	relayConfig := RelayFromConfig(ctx, cfg, crypto.ZeroPrivateKey)
	relay, err := relay.Run(ctx, relayConfig)
	if err != nil {
		cancel()
		fmt.Printf("could not open relay ports: %v\n", err)
		os.Exit(1)
	}
	nodeToken := crypto.TokenFromString(cfg.Token)
	secrets := config.WaitForRemoteKeysSync(ctx, []crypto.Token{nodeToken}, "localhost", cfg.AdminPort)
	var nodeSecret crypto.PrivateKey
	if secret, ok := secrets[nodeToken]; !ok {
		fmt.Println("Key sync failed, exiting")
		cancel()
		os.Exit(1)
	} else {
		nodeSecret = secret
	}

	adm, err := admin.OpenAdminPort(ctx, "localhost", nodeSecret, cfg.AdminPort, relayConfig.Firewall.AcceptGateway, relayConfig.Firewall.AcceptBlockListener)
	if err != nil {
		fmt.Printf("could not open admin port: %v\n", err)
		cancel()
		os.Exit(1)
	}

	if os.Args[2] == "genesis" {
		fmt.Println("creating genesis node")
		validatorConfig := swell.ValidatorConfig{
			Credentials:    nodeSecret,
			WalletPath:     cfg.WalletPath,
			SwellConfig:    swellConfig,
			Relay:          relay,
			Admin:          adm,
			TrustedGateway: TokenAddrArrayFromPeeers(cfg.TrustedNodes),
		}
		swell.NewGenesisNode(ctx, nodeSecret, validatorConfig)
		return
	} else if len(os.Args) < 5 {
		fmt.Println(usage)
		os.Exit(1)
	} else {
		tokenAddr := socket.TokenAddr{
			Addr:  os.Args[3],
			Token: crypto.TokenFromString(os.Args[4]),
		}
		if tokenAddr.Token.Equal(crypto.ZeroToken) {
			cancel()
			fmt.Printf("invalid token: %v\n%v\n", os.Args[4], usage)
			os.Exit(1)
		}
		validatorConfig := swell.ValidatorConfig{
			Credentials:    nodeSecret,
			WalletPath:     cfg.WalletPath,
			SwellConfig:    swellConfig,
			Relay:          relay,
			Admin:          adm,
			TrustedGateway: TokenAddrArrayFromPeeers(cfg.TrustedNodes),
		}
		err = swell.FullSyncValidatorNode(ctx, validatorConfig, socket.TokenAddr{}, nil)
	}
	fmt.Printf("blow node terminated: %v\n", err)
	cancel()
}

func TokenAddrFromPeer(peer config.Peer) socket.TokenAddr {
	return socket.TokenAddr{
		Addr:  peer.Address,
		Token: crypto.TokenFromString(peer.Token),
	}
}

func TokenAddrArrayFromPeeers(peers []config.Peer) []socket.TokenAddr {
	addrs := make([]socket.TokenAddr, 0)
	for _, peer := range peers {
		tokenAddr := TokenAddrFromPeer(peer)
		if !tokenAddr.Token.Equal(crypto.ZeroToken) {
			addrs = append(addrs, tokenAddr)
		}
	}
	return addrs
}

func RelayNode(ctx context.Context, config *NodeConfig, secret crypto.PrivateKey) error {
	return nil
}

func RelayFromConfig(ctx context.Context, config *NodeConfig, pk crypto.PrivateKey) relay.Config {
	listGateways := make([]crypto.Token, 0)
	for _, peer := range config.Relay.Gateway.Firewall.TokenList {
		if token := crypto.TokenFromString(peer); !token.Equal(crypto.ZeroToken) {
			listGateways = append(listGateways, token)
		}
	}
	listBlockListeners := make([]crypto.Token, 0)
	for _, peer := range config.Relay.BlockStorage.Firewall.TokenList {
		if token := crypto.TokenFromString(peer); !token.Equal(crypto.ZeroToken) {
			listBlockListeners = append(listBlockListeners, token)
		}
	}

	firewall := relay.Firewall{
		AcceptGateway:       socket.NewValidConnections(listGateways, config.Relay.Gateway.Firewall.Open),
		AcceptBlockListener: socket.NewValidConnections(listBlockListeners, config.Relay.BlockStorage.Firewall.Open),
	}

	return relay.Config{
		Credentials:       pk,
		GatewayPort:       config.Relay.Gateway.Port,
		BlockListenerPort: config.Relay.BlockStorage.Port,
		Firewall:          &firewall,
	}
}

func SwellConfigFromConfig(cfg *NodeConfig) swell.SwellNetworkConfiguration {

	swell := swell.SwellNetworkConfiguration{
		NetworkHash:      crypto.Hasher([]byte(cfg.Genesis.NetworkID)),
		MaxPoolSize:      cfg.Network.Breeze.Swell.CommitteeSize,
		MaxCommitteeSize: cfg.Network.Breeze.ChecksumCommitteeSize,
		BlockInterval:    time.Duration(cfg.Network.Breeze.BlockInterval) * time.Millisecond,
		ChecksumWindow:   cfg.Network.Breeze.ChecksumWindowBlocks,
	}
	if poa := cfg.Network.Permission.POA; poa != nil {
		tokens := make([]crypto.Token, 0)
		for _, trusted := range poa.TrustedNodes {
			var token crypto.Token
			token = crypto.TokenFromString(trusted)
			if !token.Equal(crypto.ZeroToken) {
				tokens = append(tokens, token)
			}
		}
		swell.Permission = permission.NewProofOfAuthority(tokens...)
	} else if pos := cfg.Network.Permission.POS; pos != nil {
		swell.Permission = &permission.ProofOfStake{MinimumStage: uint64(pos.MinimumStake)}
	} else {
		swell.Permission = permission.Permissionless{}
	}
	return swell
}

func CreateNodeFromGenesis(ctx context.Context, config *NodeConfig, secret crypto.PrivateKey) error {
	if config.Genesis == nil {
		return errors.New("genesis configuration not specified")
	}

	return nil
}
