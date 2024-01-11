package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/blocks"
	"github.com/freehandle/breeze/middleware/config"
	"github.com/freehandle/breeze/middleware/gateway"
)

type BeatConfig struct {
	Token       crypto.Token
	Wallet      crypto.Token
	Port        int
	AdminPort   int
	LogPath     string
	GatewayPort int
	BlocksPort  int
	Breeze      config.BreezeConfig
	Firewall    config.FirewallConfig
	Trusted     []config.Peer
}

func (b BeatConfig) Check() error {
	return nil
}

func configToGatewayConfig(cfg BeatConfig, node, wallet crypto.PrivateKey) gateway.ConfigGateway {
	return gateway.ConfigGateway{
		Credentials:     node,
		Wallet:          wallet,
		ActionPort:      cfg.GatewayPort,
		NetworkPort:     cfg.BlocksPort,
		TrustedProvider: config.PeersToTokenAddr(cfg.Trusted),
		Hostname:        "localhost",
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		os.Exit(1)
	}
	cfg, err := config.LoadConfig[EchoConfig](os.Args[1])
	if err != nil {
		fmt.Printf("configuration error: %v\n", err)
		os.Exit(1)
	}

	node := crypto.TokenFromString(cfg.Token)

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
	admin, pk := config.WaitForKeysSync(ctx, node, cfg.AdminPort)
	blocks.NewListener(ctx, admin, configToListenerConfig(*cfg, pk))
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	cancel()
}
