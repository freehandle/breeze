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
	// Token of the service... credentials will be provided by remote diffie-hellman
	Token string // `json:"token"`
	// or by a local .pem file...
	CredentialsPath string // `json:"credentialsPath"`
	// Listen Port for the service
	Port int // `json:"port"`
	// The address of the service (IP or domain name)
	Address string // `json:"address"`
	// Port for admin connections
	AdminPort int // `json:"adminPort"`
	// WalletPath should be empty for memory based wallet store
	// OR should be a path to a valid folder with appropriate permissions
	BlockRelayPort int // `json:"blockRelayPort"`

	StoragePath string // `json:"storagePath"`
	// Indexed should be true if the database should be indexed
	Indexed bool // `json:"indexed"`
	// WalletPath should be empty for memory based wallet store
	WalletPath string // `json:"walletPath"`
	// LogPath should be empty for standard logging
	// OR should be a path to a valid folder with appropriate permissions
	LogPath string // `json:"logPath"`
	// Breeze network configuration
	Breeze *config.NetworkConfig // `json:"breeze,omitempty"`

	Firewall config.FirewallConfig // `json:"firewall"`

	Trusted []config.Peer // `json:"trusted"`
}

func (b EchoConfig) Check() error {
	token := crypto.TokenFromString(b.Token)
	if token.Equal(crypto.ZeroToken) {
		return fmt.Errorf("config token is not valid")
	}
	if b.CredentialsPath != "" {
		_, err := config.ParseCredentials(b.CredentialsPath, token)
		if err != nil {
			return fmt.Errorf("could not parse credentials: %v", err)
		}
	}
	if b.Port < 0 || b.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}
	if b.AdminPort < 0 || b.AdminPort > 65535 {
		return fmt.Errorf("admin port must be between 0 and 65535")
	}
	if b.BlockRelayPort < 0 || b.BlockRelayPort > 65535 {
		return fmt.Errorf("block relay port must be between 0 and 65535")
	}
	if b.Breeze != nil {
		if err := b.Breeze.Check(); err != nil {
			return err
		}
	}
	if err := b.Firewall.Check(); err != nil {
		return err
	}
	if len(b.Trusted) == 0 {
		return fmt.Errorf("trusted peers must be specified")
	}
	return nil
}

func configToListenerConfig(cfg EchoConfig, pk crypto.PrivateKey) blocks.Config {
	listenerCfg := blocks.Config{
		Credentials: pk,
		DB: blockdb.DBConfig{
			Path:           cfg.StoragePath,
			Indexed:        cfg.Indexed,
			ItemsPerBucket: ItemsPerBucket,
			BitsForBucket:  BitsForBucket,
			IndexSize:      IndexSize,
		},
		Port:           cfg.Port,
		Firewall:       config.FirewallToValidConnections(cfg.Firewall),
		Hostname:       "localhost",
		Sources:        config.PeersToTokenAddr(cfg.Trusted),
		BlockRelayPort: cfg.BlockRelayPort,
	}
	if cfg.Breeze == nil {
		listenerCfg.Breeze = *config.StandardBreezeNetworkConfig
	} else {
		listenerCfg.Breeze = *cfg.Breeze
	}
	return listenerCfg
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
	var pk crypto.PrivateKey
	if cfg.CredentialsPath != "" {
		pk, err = config.ParseCredentials(cfg.CredentialsPath, node)
		if err != nil {
			fmt.Printf("could not retrieve credentials from file: %v\n", err)
			cancel()
			os.Exit(1)
		}
		if !node.Equal(node) {
			fmt.Println("Token in credentials file does not match token in config")
			cancel()
			os.Exit(1)
		}
	} else {
		secrets := config.WaitForRemoteKeysSync(ctx, []crypto.Token{node}, "localhost", cfg.AdminPort)
		if secret, ok := secrets[node]; !ok {
			fmt.Println("Key sync failed, exiting")
			cancel()
			os.Exit(1)
		} else {
			pk = secret
		}
	}

	listenerCfg := configToListenerConfig(*cfg, pk)
	adm, err := admin.OpenAdminPort(ctx, "localhost", pk, cfg.AdminPort, nil, listenerCfg.Firewall)
	if err != nil {
		fmt.Printf("could not open admin port: %v\n", err)
		cancel()
		os.Exit(1)
	}
	blocks.NewServer(ctx, adm, listenerCfg)
	fmt.Println("server is running")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	cancel()
}
