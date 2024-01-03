package main

import (
	"errors"
	"log"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

const (
	RegisteredNodeKind byte = iota
	WalletKeyKind
)

type RegisteredNode struct {
	ID          string
	Host        string
	Token       crypto.Token
	Description string
	Live        bool
}

func ParseRegisteredNode(data []byte) *RegisteredNode {
	if len(data) == 0 || data[0] != RegisteredNodeKind {
		return nil
	}
	position := 1
	node := RegisteredNode{}
	node.ID, position = util.ParseString(data, position)
	node.Host, position = util.ParseString(data, position)
	node.Token, position = util.ParseToken(data, position)
	node.Description, position = util.ParseString(data, position)
	node.Live, position = util.ParseBool(data, position)
	if position != len(data) {
		return nil
	}
	return &node
}

func (r RegisteredNode) Serialize() []byte {
	data := []byte{RegisteredNodeKind}
	util.PutString(r.ID, &data)
	util.PutString(r.Host, &data)
	util.PutToken(r.Token, &data)
	util.PutString(r.Description, &data)
	util.PutBool(r.Live, &data)
	return data
}

type WalletKey struct {
	Secret      crypto.PrivateKey
	Description string
	ID          string
}

func ParseWalletKey(data []byte) *WalletKey {
	if len(data) == 0 || data[0] != WalletKeyKind {
		return nil
	}
	position := 1
	wallet := WalletKey{}
	wallet.Secret, position = util.ParseSecret(data, position)
	wallet.Description, position = util.ParseString(data, position)
	wallet.ID, position = util.ParseString(data, position)
	if position != len(data) {
		return nil
	}
	return &wallet
}

func (w WalletKey) Serialize() []byte {
	data := []byte{WalletKeyKind}
	util.PutSecret(w.Secret, &data)
	util.PutString(w.Description, &data)
	util.PutString(w.ID, &data)
	return data
}

type Safe struct {
	Nodes      []RegisteredNode
	WalletKeys []WalletKey
	vault      *util.SecureVault
}

func (s *Safe) Close() {
	s.vault.Close()
}

func (safe *Safe) SecureItem(data []byte) error {
	return safe.vault.NewEntry(data)
}

func (vault *Safe) GenerateNewKey(id, description string) (crypto.Token, crypto.PrivateKey) {
	for _, wallet := range vault.WalletKeys {
		if wallet.ID == id {
			log.Fatal("Wallet ID already exists")
		}
	}
	token, newKey := crypto.RandomAsymetricKey()
	key := WalletKey{
		Secret:      newKey,
		Description: description,
		ID:          id,
	}
	vault.SecureItem(key.Serialize())
	vault.WalletKeys = append(vault.WalletKeys, key)
	return token, newKey
}

func (vault *Safe) RegisteredNode(id, host, description string, token crypto.Token) error {
	exists := false
	for _, node := range vault.Nodes {
		if node.ID == id {
			exists = node.Live
		}
	}
	if exists {
		return errors.New("Node already exists. remove it first")
	}
	node := RegisteredNode{
		ID:          id,
		Host:        host,
		Token:       token,
		Description: description,
		Live:        true,
	}
	vault.SecureItem(node.Serialize())
	vault.Nodes = append(vault.Nodes, node)
	return nil
}

func (vault *Safe) RemoveNode(id string) error {
	exists := false
	for _, node := range vault.Nodes {
		if node.ID == id {
			exists = node.Live
		}
	}
	if !exists {
		return errors.New("No registered node found.")
	}
	node := RegisteredNode{
		ID:   id,
		Live: false,
	}
	vault.SecureItem(node.Serialize())
	vault.Nodes = append(vault.Nodes, node)
	return nil
}

func NewSecureVault(password []byte, fileName string) (*Safe, error) {
	vault, err := util.NewSecureVault(password, fileName)
	if err != nil {
		return nil, err
	}
	safe := Safe{
		Nodes:      make([]RegisteredNode, 0),
		WalletKeys: make([]WalletKey, 0),
		vault:      vault,
	}
	return &safe, nil
}

func OpenVaultFromPassword(password []byte, fileName string) (*Safe, error) {
	vault, err := util.OpenVaultFromPassword(password, fileName)
	if err != nil {
		return nil, err
	}

	safe := Safe{
		Nodes:      make([]RegisteredNode, 0),
		WalletKeys: make([]WalletKey, 0),
		vault:      vault,
	}
	for _, entry := range vault.Entries {
		if len(entry) == 0 {
			continue
		} else if entry[0] == RegisteredNodeKind {
			node := ParseRegisteredNode(entry)
			if node != nil {
				safe.Nodes = append(safe.Nodes, *node)
			} else {
				return nil, errors.New("could not parse node")
			}
		} else if entry[0] == WalletKeyKind {
			wallet := ParseWalletKey(entry)
			if wallet != nil {
				safe.WalletKeys = append(safe.WalletKeys, *wallet)
			} else {
				return nil, errors.New("could not parse wallet key")
			}
		}
	}
	return &safe, nil
}
