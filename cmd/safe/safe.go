package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

const (
	RegisteredNodeKind byte = iota
	WalletKeyKind
	DefaultKind
)

type RegisteredNode struct {
	ID          string
	Host        string
	Token       crypto.Token
	Description string
	Live        bool
}

func ParseDefault(data []byte) (string, byte) {
	if len(data) < 2 {
		return "", 0
	}
	position := 1
	var node string
	var scope byte
	node, position = util.ParseString(data, 1)
	scope, position = util.ParseByte(data, position)
	if position != len(data) {
		return "", 0
	}
	return node, scope
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
	Gateway    string
	Listener   string
	vault      *util.SecureVault
}

func (s *Safe) findNode(id string) RegisteredNode {
	var found RegisteredNode
	for _, node := range s.Nodes {
		if node.ID == id {
			found = node
		}
	}
	return found
}

func (s *Safe) findSecret(token crypto.Token) crypto.PrivateKey {
	if s.vault.SecretKey.PublicKey().Equal(token) {
		return s.vault.SecretKey
	}
	for _, wallet := range s.WalletKeys {
		if wallet.Secret.PublicKey().Equal(token) {
			return wallet.Secret
		}
	}
	return crypto.ZeroPrivateKey
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

func (safe *Safe) DefaultNode(id string, gateway bool) {
	msg := []byte{DefaultKind}
	util.PutString(id, &msg)
	if gateway {
		msg = append(msg, 1)
	} else {
		msg = append(msg, 0)
	}
	safe.SecureItem(msg)
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
		} else if entry[1] == DefaultKind {
			node, scope := ParseDefault(entry)
			if node != "" {
				if scope == 0 {
					safe.Listener = node
				} else {
					safe.Gateway = node
				}
			} else {
				return nil, errors.New("could not parse default node")
			}
		}
	}
	return &safe, nil
}

func (safe *Safe) dialGateway() (*socket.SignedConnection, uint64, error) {
	if safe.Gateway == "" {
		return nil, 0, errors.New("no gateway configured")
	}
	node := safe.findNode(safe.Gateway)
	if node.ID == "" {
		return nil, 0, errors.New("configured gateway not found")
	}
	conn, err := socket.Dial("localhost", node.Host, safe.vault.SecretKey, node.Token)
	if err != nil {
		return nil, 0, err
	}
	epochData, err := conn.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("could not read from gateway: %s", err)
	}
	epoch, _ := util.ParseUint64(epochData, 0)
	return conn, epoch, nil
}
