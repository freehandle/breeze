package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/admin"
	"github.com/freehandle/breeze/middleware/config"
	"github.com/freehandle/breeze/middleware/gateway"
)

const usage = `usage: beat <config.json>`

type BeatConfig struct {
	Token           string
	Wallet          string
	Port            int
	AdminPort       int
	LogPath         string
	ActionRelayPort int
	BlockRelayPort  int
	Breeze          config.BreezeConfig
	Firewall        config.FirewallConfig
	Trusted         []config.Peer
}

func (b BeatConfig) Check() error {
	return nil
}

func configToGatewayConfig(cfg BeatConfig, node, wallet crypto.PrivateKey) gateway.Configuration {
	return gateway.Configuration{
		Credentials:     node,
		Wallet:          wallet,
		Hostname:        "localhost",
		Port:            cfg.Port,
		Firewall:        config.FirewallToValidConnections(cfg.Firewall),
		Trusted:         config.PeersToTokenAddr(cfg.Trusted),
		ActionRelayPort: cfg.ActionRelayPort,
		BlockRelayPort:  cfg.BlockRelayPort,
		Breeze:          cfg.Breeze,
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		os.Exit(1)
	}
	cfg, err := config.LoadConfig[BeatConfig](os.Args[1])
	if err != nil {
		fmt.Printf("configuration error: %v\n", err)
		os.Exit(1)
	}

	node := crypto.TokenFromString(cfg.Token)
	wallet := crypto.TokenFromString(cfg.Wallet)
	if wallet.Equal(crypto.ZeroToken) {
		wallet = node
	}

	if cfg.LogPath != "" {
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

	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tokens := []crypto.Token{node}
	if !wallet.Equal(node) {
		tokens = append(tokens, wallet)
	}

	keys := config.WaitForRemoteKeysSync(ctx, tokens, "localhost", cfg.AdminPort)

	nodeSecret := keys[node]
	walletSecret := keys[wallet]

	if !nodeSecret.PublicKey().Equal(node) {
		fmt.Println("node secret key is not valid")
		os.Exit(1)
	}
	if !walletSecret.PublicKey().Equal(wallet) {
		fmt.Println("wallet secret key is not valid")
		os.Exit(1)
	}
	gatewayCfg := configToGatewayConfig(*cfg, nodeSecret, walletSecret)

	adm, err := admin.OpenAdminPort(ctx, "localhost", nodeSecret, cfg.AdminPort, gatewayCfg.Firewall, gatewayCfg.Firewall)
	if err != nil {
		fmt.Printf("could not open admin port: %v\n", err)
		cancel()
		os.Exit(1)
	}

	gateway.NewServer(ctx, gatewayCfg, adm)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	for {
		done := ctx.Done()
		select {
		case <-done:
			return
		case <-c:
			return
		case instruction := <-adm.Interaction:
			if len(instruction.Request) == 2 && instruction.Request[0] == admin.MsgActivation && instruction.Request[1] == 1 {
				return
			}
		}
	}
}
