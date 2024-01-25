package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/admin"
	"github.com/freehandle/breeze/middleware/config"
	"github.com/freehandle/breeze/socket"
)

const usage = "usage: blow <path-to-json-config-file> [genesis|sync address token|check]"

func main() {
	var err error
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

	if os.Args[2] == "check" {
		bytes, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Printf("config file:\n %s\n", string(bytes))
		return
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

	ctx, cancel := context.WithCancel(context.Background())

	nodeToken := crypto.TokenFromString(cfg.Token)
	var nodeSecret crypto.PrivateKey
	if cfg.CredentialsPath != "" {
		nodeSecret, err = config.ParseCredentials(cfg.CredentialsPath, nodeToken)
		if err != nil {
			fmt.Printf("could not retrieve credentials from file: %v\n", err)
			cancel()
			os.Exit(1)
		}
	} else {
		secrets := config.WaitForRemoteKeysSync(ctx, []crypto.Token{nodeToken}, "localhost", cfg.AdminPort)
		if secret, ok := secrets[nodeToken]; !ok {
			fmt.Println("Key sync failed, exiting")
			cancel()
			os.Exit(1)
		} else {
			nodeSecret = secret
		}

	}

	swellConfig := config.SwellConfigFromConfig(cfg.Network, cfg.Genesis.NetworkID)
	relayConfig := RelayFromConfig(ctx, cfg, crypto.ZeroPrivateKey)
	relay, err := relay.Run(ctx, relayConfig)
	if err != nil {
		cancel()
		fmt.Printf("could not open relay ports: %v\n", err)
		os.Exit(1)
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
	for _, peer := range config.Relay.Blocks.Firewall.TokenList {
		if token := crypto.TokenFromString(peer); !token.Equal(crypto.ZeroToken) {
			listBlockListeners = append(listBlockListeners, token)
		}
	}

	firewall := relay.Firewall{
		AcceptGateway:       socket.NewValidConnections(listGateways, config.Relay.Gateway.Firewall.Open),
		AcceptBlockListener: socket.NewValidConnections(listBlockListeners, config.Relay.Blocks.Firewall.Open),
	}

	return relay.Config{
		Credentials:       pk,
		GatewayPort:       config.Relay.Gateway.Port,
		BlockListenerPort: config.Relay.Blocks.Port,
		Firewall:          &firewall,
	}
}

func CreateNodeFromGenesis(ctx context.Context, config *NodeConfig, secret crypto.PrivateKey) error {
	if config.Genesis == nil {
		return errors.New("genesis configuration not specified")
	}

	return nil
}
