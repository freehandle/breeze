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
	config, err := config.LoadConfig[NodeConfig](os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	log := filepath.Join(config.LogPath, fmt.Sprintf("%v.log", config.Token[0:16]))

	logFile, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("could not open log file: %v\n", err)
		os.Exit(1)
	}
	var programLevel = new(slog.LevelVar) // Info by default
	programLevel.Set(slog.LevelDebug)
	logger := slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: programLevel})
	slog.SetDefault(slog.New(logger))

	swellConfig := SwellConfigFromConfig(config)
	relay, err := RelayFromConfig(ctx, config, crypto.ZeroPrivateKey)
	if err != nil {
		cancel()
		fmt.Printf("could not open relay ports: %v\n", err)
		os.Exit(1)
	}
	if os.Args[2] == "genesis" {
		fmt.Println("creating genesis node")
		admin, pk := WaitForKeysSync(ctx, config)
		if admin == nil {
			cancel()
			fmt.Println("canceled")
			os.Exit(1)
		}

		validatorConfig := swell.ValidatorConfig{
			Credentials:    pk,
			WalletPath:     config.WalletPath,
			SwellConfig:    swellConfig,
			Relay:          relay,
			Admin:          admin,
			TrustedGateway: TokenAddrArrayFromPeeers(config.TrustedNodes),
		}
		swell.NewGenesisNode(ctx, pk, validatorConfig)
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
		admin, pk := WaitForKeysSync(ctx, config)
		validatorConfig := swell.ValidatorConfig{
			Credentials:    pk,
			WalletPath:     config.WalletPath,
			SwellConfig:    swellConfig,
			Relay:          relay,
			Admin:          admin,
			TrustedGateway: TokenAddrArrayFromPeeers(config.TrustedNodes),
		}
		if admin == nil {
			cancel()
			fmt.Println("canceled")
			os.Exit(1)
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

func RelayFromConfig(ctx context.Context, config *NodeConfig, pk crypto.PrivateKey) (*relay.Node, error) {
	listGateways := make([]crypto.Token, 0)
	for _, peer := range config.Relay.Gateway.Firewall.Whitelist {
		if token := crypto.TokenFromString(peer); !token.Equal(crypto.ZeroToken) {
			listGateways = append(listGateways, token)
		}
	}
	listBlockListeners := make([]crypto.Token, 0)
	for _, peer := range config.Relay.BlockStorage.Firewall.Whitelist {
		if token := crypto.TokenFromString(peer); !token.Equal(crypto.ZeroToken) {
			listBlockListeners = append(listBlockListeners, token)
		}
	}

	firewall := relay.Firewall{
		AcceptGateway:       socket.NewValidConnections(listGateways, config.Relay.Gateway.Firewall.OpenRelay),
		AcceptBlockListener: socket.NewValidConnections(listBlockListeners, config.Relay.BlockStorage.Firewall.OpenRelay),
	}

	relayConfig := relay.Config{
		Credentials:       pk,
		GatewayPort:       config.Relay.Gateway.Port,
		BlockListenerPort: config.Relay.BlockStorage.Port,
		Firewall:          &firewall,
	}
	return relay.Run(ctx, relayConfig)
}

func SwellConfigFromConfig(config *NodeConfig) swell.SwellNetworkConfiguration {
	swell := swell.SwellNetworkConfiguration{
		NetworkHash:      crypto.Hasher([]byte(config.Genesis.NetworkID)),
		MaxPoolSize:      config.Breeze.Swell.CommitteeSize,
		MaxCommitteeSize: config.Breeze.ChecksumCommitteeSize,
		BlockInterval:    time.Duration(config.Breeze.BlockInterval) * time.Millisecond,
		ChecksumWindow:   config.Breeze.ChecksumWindowBlocks,
	}
	if poa := config.Breeze.Permission.POA; poa != nil {
		tokens := make([]crypto.Token, 0)
		for _, trusted := range poa.TrustedNodes {
			var token crypto.Token
			token = crypto.TokenFromString(trusted)
			if !token.Equal(crypto.ZeroToken) {
				tokens = append(tokens, token)
			}
		}
		swell.Permission = permission.NewProofOfAuthority(tokens...)
	} else if pos := config.Breeze.Permission.POS; pos != nil {
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
