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
	"github.com/freehandle/breeze/middleware/blockdb"
	"github.com/freehandle/breeze/middleware/blocks"
	"github.com/freehandle/breeze/middleware/config"
)

const usage = "usage: echo <path-to-json-config-file>"

var (
	ItemsPerBucket = 10
	BitsForBucket  = 10
	IndexSize      = 8
)

// Echoconfig is the configuration for an echo service
type EchoConfig struct {
	// Token of the service... credentials will be provided by diffie-hellman
	Token string
	Port  int
	// The address of the service (IP or domain name)
	Address string
	// Port for admin connections
	AdminPort int
	// WalletPath should be empty for memory based wallet store
	// OR should be a path to a valid folder with appropriate permissions
	StoragePath string

	Indexed bool

	WalletPath string
	// LogPath should be empty for standard logging
	// OR should be a path to a valid folder with appropriate permissions
	LogPath string
	// Breeze network configuration
	Breeze config.BreezeConfig

	Firewall config.FirewallConfig

	TrustedNode []config.Peer
}

func (cfg EchoConfig) Check() error {
	return nil
}

func configToListenerConfig(cfg EchoConfig, pk crypto.PrivateKey) blocks.Config {
	return blocks.Config{
		Credentials: pk,
		DB: blockdb.DBConfig{
			Path:           cfg.StoragePath,
			Indexed:        cfg.Indexed,
			ItemsPerBucket: ItemsPerBucket,
			BitsForBucket:  BitsForBucket,
			IndexSize:      IndexSize,
		},
		Port:     cfg.Port,
		Firewall: config.FirewallToValidConnections(cfg.Firewall),
		Hostname: "localhost",
		Sources:  config.PeersToTokenAddr(cfg.TrustedNode),
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
	secrets := config.WaitForRemoteKeysSync(ctx, []crypto.Token{node}, "localhost", cfg.AdminPort)
	var pk crypto.PrivateKey
	if secret, ok := secrets[node]; !ok {
		fmt.Println("Key sync failed, exiting")
		cancel()
		os.Exit(1)
	} else {
		pk = secret
	}
	listenerCfg := configToListenerConfig(*cfg, pk)
	adm, err := admin.OpenAdminPort(ctx, "localhost", pk, cfg.AdminPort, nil, listenerCfg.Firewall)
	if err != nil {
		fmt.Printf("could not open admin port: %v\n", err)
		cancel()
		os.Exit(1)
	}
	blocks.NewBreezeListener(ctx, adm, listenerCfg)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	cancel()
}
