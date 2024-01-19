package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/admin"
	"github.com/freehandle/breeze/socket"
)

func parseCommandArgs(cmd byte, args []string) Command {
	switch cmd {
	case createCmd:
		return &CreateCommand{}
	case showCmd:
		return &ShowCommand{}
	case statusCmd:
		if len(args) < 1 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &StatusCommand{
			NodeId: args[0],
		}
	case syncCmd:
		if len(args) < 3 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &SyncCommand{
			Address:   args[0],
			TempToken: args[1],
			Token:     args[2],
		}
	case registerCmd:
		if len(args) < 4 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &RegisterCommand{
			NodeID:      args[0],
			Address:     args[1],
			Token:       args[2],
			Description: args[3],
		}
	case removeCmd:
		if len(args) < 1 {
			return nil
		}
		return &RemoveCommand{
			NodeID: args[0],
		}
	case nodesCmd:
		return &NodesCommand{}
	case transferCmd:
		if len(args) < 4 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &TransferCommand{
			From:    args[0],
			To:      args[1],
			Ammount: args[2],
			Fee:     args[3],
		}
	case depositCmd:
		if len(args) < 3 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &StakeCommand{
			Account: args[0],
			Ammount: args[1],
			Fee:     args[2],
			Deposit: true,
		}
	case withdrawCmd:
		if len(args) < 3 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &StakeCommand{
			Account: args[0],
			Ammount: args[1],
			Fee:     args[2],
			Deposit: false,
		}
	case balanceCmd:
		if len(args) < 1 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &BalanceCommand{
			Account: args[0],
		}
	case configCmd:
		if len(args) < 2 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &ConfigCommand{
			Variable: args[0],
			NodeID:   args[1],
		}
	case grantCmd:
		if len(args) < 3 {
			fmt.Println("insufficient arguments")
			return nil
		}
		scope := strings.ToLower(args[2])
		if scope != "gateway" && scope != "block" {
			fmt.Println("wrong scope")
			return nil
		}
		return &FirewallCommand{
			Include: true,
			NodeId:  args[0],
			Token:   args[1],
			Gateway: scope == "gateway",
		}
	case revokeCmd:
		if len(args) < 3 {
			fmt.Println("insufficient arguments")
			return nil
		}
		scope := strings.ToLower(args[2])
		if scope != "gateway" && scope != "block" {
			fmt.Println("wrong scope")
			return nil
		}
		return &FirewallCommand{
			Include: false,
			NodeId:  args[0],
			Token:   args[1],
			Gateway: scope == "gateway",
		}
	case activityCmd:
		if len(args) < 2 {
			fmt.Println("insufficient arguments")
			return nil
		}
		status := strings.ToLower(args[1])
		if status != "activate" && status != "deactivate" {
			fmt.Println("wrong status")
		}
		return &ActivityCommand{
			NodeId:   args[0],
			Activate: status == "activate",
		}
	default:
		return nil
	}
}

type Command interface {
	Execute(*Kite) error
}

type ActivityCommand struct {
	NodeId   string
	Activate bool
}

func (f *ActivityCommand) Execute(safe *Kite) error {
	node, err := dialAdmin(safe, f.NodeId)
	if err != nil {
		return err
	}
	err = node.Activity(f.Activate)
	if err != nil {
		return fmt.Errorf("could not reset status: %v", err)
	}
	return nil
}

type FirewallCommand struct {
	Include bool
	NodeId  string
	Token   string
	Gateway bool
}

func (f *FirewallCommand) Execute(safe *Kite) error {
	token := crypto.TokenFromString(f.Token)
	if token.Equal(crypto.ZeroToken) {
		return fmt.Errorf("invalid token: %v", f.Token)
	}
	adm, err := dialAdmin(safe, f.NodeId)
	if err != nil {
		return err
	}
	var scope byte
	if f.Include {
		if f.Gateway {
			scope = admin.GrantGateway
		} else {
			scope = admin.GrantBlockListener
		}
	} else {
		if f.Gateway {
			scope = admin.RevokeGateway
		} else {
			scope = admin.RevokeBlockListener
		}
	}
	err = adm.FirewallAction(scope, token)
	if err != nil {
		return fmt.Errorf("could not get status: %v", err)
	}
	return nil

}

type SyncCommand struct {
	Address   string
	TempToken string
	Token     string
}

func (c *SyncCommand) Execute(safe *Kite) error {
	tempToken := crypto.TokenFromString(c.TempToken)
	token := crypto.TokenFromString(c.Token)
	if tempToken.Equal(crypto.ZeroToken) {
		return fmt.Errorf("invalid token: %v", c.TempToken)
	}
	if token.Equal(crypto.ZeroToken) {
		return fmt.Errorf("invalid token: %v", c.Token)
	}
	var secretKey crypto.PrivateKey
	if safe.vault.SecretKey.PublicKey().Equal(token) {
		secretKey = safe.vault.SecretKey
	} else {
		for _, wallet := range safe.WalletKeys {
			if wallet.Secret.PublicKey().Equal(token) {
				secretKey = wallet.Secret
				break
			}
		}

	}
	if secretKey == crypto.ZeroPrivateKey {
		return fmt.Errorf("secret for token %s not found", token)
	}
	tokenAddr := socket.TokenAddr{
		Addr:  c.Address,
		Token: tempToken,
	}
	admin, err := admin.DialAdmin("localhost", tokenAddr, safe.vault.SecretKey)
	if err != nil {
		return fmt.Errorf("could not connect to node: %v", err)
	}
	err = admin.SendSecret(secretKey)
	if err != nil {
		return fmt.Errorf("could not exchange key: %v", err)
	}
	return nil
}

type StatusCommand struct {
	NodeId string
}

func (s StatusCommand) Execute(safe *Kite) error {
	var node RegisteredNode
	for _, n := range safe.Nodes {
		if n.ID == s.NodeId && n.Live {
			node = n
		}
	}
	if node.ID == "" {
		return fmt.Errorf("node %s not found", s.NodeId)
	}
	tokenAddr := socket.TokenAddr{
		Addr:  node.Host,
		Token: node.Token,
	}
	admin, err := admin.DialAdmin("localhost", tokenAddr, safe.vault.SecretKey)
	if err != nil {
		return fmt.Errorf("could not connect to admin node %v: %v", tokenAddr.Token, err)
	}
	status, err := admin.Status()
	if err != nil {
		return fmt.Errorf("could not get status: %v", err)
	}
	fmt.Printf("node %s status:\n%s\n", node.ID, status)
	return nil
}

type ShowCommand struct{}

func (c *ShowCommand) Execute(vault *Kite) error {
	secret := hex.EncodeToString(vault.vault.SecretKey[:])
	fmt.Printf("Vault token: %v\nSecret: %v\n", vault.vault.SecretKey.PublicKey(), secret)

	return nil
}

type CreateCommand struct{}

func (c *CreateCommand) Execute(vault *Kite) error {
	if vault != nil {
		return errors.New("vault already exists")
	}
	password := readPassword("Enter pass phrase to secure safe vault:")
	password2 := readPassword("Reenter pass phrase to secure safe vault:")
	if string(password) != string(password2) {
		return errors.New("passwords do not match")
	}
	var err error
	vault, err = NewSecureVault([]byte(password), os.Args[1])
	if vault == nil {
		return fmt.Errorf("could not create vault: %v", err)
	}
	return nil
}

// 1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef

type RegisterCommand struct {
	NodeID      string
	Address     string
	Token       string
	Description string
}

func (c *RegisterCommand) Execute(safe *Kite) error {
	token := crypto.TokenFromString(c.Token)
	if token.Equal(crypto.ZeroToken) {
		return fmt.Errorf("invalid token: %v", c.Token)
	}
	//conn, err := socket.Dial("localhost", c.Address, safe.vault.SecretKey, token)
	//if err != nil {
	//	return fmt.Errorf("could not connect to node: %v", err)
	//}
	//conn.Shutdown()
	return safe.RegisteredNode(c.NodeID, c.Address, c.Description, token)
}

type RemoveCommand struct {
	NodeID string
}

func (c *RemoveCommand) Execute(vault *Kite) error {
	return vault.RemoveNode(c.NodeID)
}

type NodesCommand struct{}

func (c *NodesCommand) Execute(vault *Kite) error {
	live := make(map[string]RegisteredNode)
	for _, node := range vault.Nodes {
		if _, ok := live[node.ID]; !ok {
			if node.Live {
				live[node.ID] = node
			}
		} else {
			if !node.Live {
				delete(live, node.ID)
			}
		}
	}
	for _, node := range live {
		fmt.Printf("%s\t%s\t%s\t%s\n", node.ID, node.Host, node.Token, node.Description)
	}
	return nil
}

type ConfigCommand struct {
	Variable string
	NodeID   string
}

func (c *ConfigCommand) Execute(vault *Kite) error {
	return nil
}

func dialAdmin(safe *Kite, nodeID string) (*admin.AdminClient, error) {
	node := safe.findNode(nodeID)
	if node.ID == "" {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}
	tokenAddr := socket.TokenAddr{
		Addr:  node.Host,
		Token: node.Token,
	}
	admin, err := admin.DialAdmin("localhost", tokenAddr, safe.vault.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("could not connect to admin node %v: %v", tokenAddr.Token, err)
	}
	return admin, nil
}
