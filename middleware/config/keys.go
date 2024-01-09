package config

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/freehandle/breeze/consensus/admin"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

func WaitForKeysSync(ctx context.Context, node crypto.Token, port int) (*admin.Administration, crypto.PrivateKey) {
	token, pk := crypto.RandomAsymetricKey()
	fmt.Printf("waiting for secrete key for token\n\n        %v\n\non admin port %v with provisory token\n\n        %v\n\n", node, port, token)
	administration := &admin.Administration{
		Firewall:       socket.ValidateSingleConnection(token),
		Secret:         pk,
		Port:           port,
		Status:         make(chan chan string),
		Activation:     make(chan admin.Activation),
		FirewallAction: make(chan admin.FirewallAction),
	}
	nodeSecret, err := administration.WaitForKeys(ctx, node)
	if err != nil {
		slog.Info("could not get secret key for node: %v", err)
		return nil, crypto.ZeroPrivateKey
	}
	return administration, nodeSecret
}
