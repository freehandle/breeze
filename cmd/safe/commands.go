package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/freehandle/breeze/crypto"
)

func parseCommandArgs(cmd byte, args []string) Command {
	switch cmd {
	case createCmd:
		return &CreateCommand{}
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
		if len(args) < 3 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &TransferCommand{
			TokenAmount: args[0],
			FromAccount: args[1],
			ToAccount:   args[2],
		}
	case depositCmd:
		if len(args) < 2 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &DepositCommand{
			TokenAmount: args[0],
			Account:     args[1],
		}
	case withdrawCmd:
		if len(args) < 2 {
			fmt.Println("insufficient arguments")
			return nil
		}
		return &WithdrawCommand{
			TokenAmount: args[0],
			Account:     args[1],
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

	default:
		return nil
	}
}

type Command interface {
	Execute(*Safe) error
}

type CreateCommand struct{}

func (c *CreateCommand) Execute(vault *Safe) error {
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

func (c *RegisterCommand) Execute(safe *Safe) error {
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

func (c *RemoveCommand) Execute(vault *Safe) error {
	return vault.RemoveNode(c.NodeID)
}

type NodesCommand struct{}

func (c *NodesCommand) Execute(vault *Safe) error {
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

type TransferCommand struct {
	TokenAmount string
	FromAccount string
	ToAccount   string
}

func (c *TransferCommand) Execute(vault *Safe) error {
	return nil
}

type DepositCommand struct {
	TokenAmount string
	Account     string
}

func (c *DepositCommand) Execute(vault *Safe) error {
	return nil
}

type WithdrawCommand struct {
	TokenAmount string
	Account     string
}

func (c *WithdrawCommand) Execute(vault *Safe) error {
	return nil
}

type BalanceCommand struct {
	Account string
}

func (c *BalanceCommand) Execute(vault *Safe) error {
	return nil
}

type ConfigCommand struct {
	Variable string
	NodeID   string
}

func (c *ConfigCommand) Execute(vault *Safe) error {
	return nil
}
