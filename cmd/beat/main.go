package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/freehandle/breeze/crypto"
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

	gateway.NewServer(ctx, configToGatewayConfig(*cfg, pk, pk), admin)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	for {
		select {
		case <-c:
			cancel()
		case activation := <-admin.Activation:
			if !activation.Active {
				slog.Info("beat: received deactivation")
				cancel()
			}
		}
	}
	slog.Info("service terminated")
}
