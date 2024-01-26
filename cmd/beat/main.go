package main

import (
	"context"
	"encoding/json"
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
	Token                 string                // `json:"token"`
	CredentialsPath       string                // `json:"credentialsPath"`
	Wallet                string                // `json:"wallet,omitempty"`
	WalletCredentialsPath string                // `json:"credentialsPath,omitempty"`
	Port                  int                   // `json:"port"`
	AdminPort             int                   // `json:"adminPort"`
	LogPath               string                // `json:"logPath"`
	ActionRelayPort       int                   // `json:"actionRelayPort"`
	BlockRelayPort        int                   // `json:"blockRelayPort"`
	Breeze                *config.BreezeConfig  // `json:"breeze,omitempty"`
	Firewall              config.FirewallConfig // `json:"firewall"`
	Trusted               []config.Peer         // `json:"trusted"`
}

func (b BeatConfig) Check() error {
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
	if b.Wallet != "" {
		wallet := crypto.TokenFromString(b.Wallet)
		if wallet.Equal(crypto.ZeroToken) {
			return fmt.Errorf("wallet token is not valid")
		}
		if b.WalletCredentialsPath != "" {
			_, err := config.ParseCredentials(b.WalletCredentialsPath, wallet)
			if err != nil {
				return fmt.Errorf("could not parse wallet credentials: %v", err)
			}
		}
	}
	if b.Port < 0 || b.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}
	if b.AdminPort < 0 || b.AdminPort > 65535 {
		return fmt.Errorf("admin port must be between 0 and 65535")
	}
	if b.ActionRelayPort < 0 || b.ActionRelayPort > 65535 {
		return fmt.Errorf("action relay port must be between 0 and 65535")
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
		Breeze:          *cfg.Breeze,
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
	if cfg.Breeze == nil {
		cfg.Breeze = config.StandardBreezeConfig
	}

	if len(os.Args) > 2 && os.Args[2] == "check" {
		bytes, _ := json.MarshalIndent(*cfg, "", "\t")
		fmt.Printf("configuration:\n%v\n", string(bytes))
		return
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

	var nodeSecret, walletSecret crypto.PrivateKey

	tokens := make([]crypto.Token, 0)
	if cfg.CredentialsPath != "" {
		nodeSecret, err = config.ParseCredentials(cfg.CredentialsPath, node)
		if err != nil {
			fmt.Printf("could not retrieve credentials from file: %v\n", err)
			cancel()
			os.Exit(1)
		}
		if wallet.Equal(node) {
			walletSecret = nodeSecret
		}
	} else {
		tokens = append(tokens, wallet)
	}
	if cfg.WalletCredentialsPath != "" {
		walletSecret, err = config.ParseCredentials(cfg.WalletCredentialsPath, wallet)
		if err != nil {
			fmt.Printf("could not retrieve credentials from file: %v\n", err)
			cancel()
			os.Exit(1)
		}
	} else if !wallet.Equal(node) {
		tokens = append(tokens, wallet)
	}

	if len(tokens) > 0 {
		keys := config.WaitForRemoteKeysSync(ctx, tokens, "localhost", cfg.AdminPort)
		for _, token := range tokens {
			if pk, ok := keys[token]; !ok || !pk.PublicKey().Equal(token) {
				fmt.Println("Key sync failed, exiting")
				cancel()
				os.Exit(1)
			}
			if token.Equal(node) {
				nodeSecret = keys[node]
			}
			if token.Equal(wallet) {
				walletSecret = keys[wallet]
			}
		}
	}

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

	fmt.Println("beat is running")

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
