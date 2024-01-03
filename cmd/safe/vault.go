package main

import (
	"crypto/rand"
	"io"
	"log"
	"os"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/crypto/scrypt"
	"github.com/freehandle/breeze/util"
)

const (
	RegisteredNodeKind byte = iota
	WalletKeyKind
)

type RegisteredNode struct {
	Host        string
	Token       crypto.Token
	Description string
	Live        bool
}

func (r RegisteredNode) Serialize() []byte {
	data := []byte{RegisteredNodeKind}
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

func (w WalletKey) Serialize() []byte {
	data := []byte{WalletKeyKind}
	util.PutSecret(w.Secret, &data)
	util.PutString(w.Description, &data)
	util.PutString(w.ID, &data)
	return data
}

type SecureVault struct {
	SecretKey  crypto.PrivateKey
	Nodes      []RegisteredNode
	WalletKeys []WalletKey
	file       io.WriteCloser
	cipher     crypto.Cipher
}

func (s *SecureVault) Close() {
	s.file.Close()
}

func (vault *SecureVault) SecureItem(data []byte) {
	if len(data) < 1 {
		log.Fatal("Invalid data to vault")
		os.Exit(1)
	}
	sealed := vault.cipher.Seal(data[1:])
	size := len(sealed) + 1
	data = append([]byte{byte(size), byte(size >> 8), data[0]}, sealed...)
	if n, err := vault.file.Write(data); n != len(data) || err != nil {
		log.Fatalf("secret vault is possibly compromissed: %v\n", err)
	}
}

func (vault *SecureVault) GenerateNewKey(id, description string) (crypto.Token, crypto.PrivateKey) {
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

func (vault *SecureVault) RegisteredNode(host, description string, token crypto.Token) {
	exists := false
	for _, node := range vault.Nodes {
		if node.Host == host {
			exists = node.Live
		}
	}
	if exists {
		log.Fatal("Node already exists. remove it first")
	}
	node := RegisteredNode{
		Host:        host,
		Token:       token,
		Description: description,
		Live:        true,
	}
	vault.SecureItem(node.Serialize())
	vault.Nodes = append(vault.Nodes, node)
}

func (vault *SecureVault) RemoveNode(host, description string, token crypto.Token) {
	exists := false
	for _, node := range vault.Nodes {
		if node.Host == host {
			exists = node.Live
		}
	}
	if exists {
		log.Fatal("Node already exists. remove it first")
	}
	node := RegisteredNode{
		Host:        host,
		Token:       token,
		Description: description,
		Live:        true,
	}
	vault.SecureItem(node.Serialize())
	vault.Nodes = append(vault.Nodes, node)
}

func NewSecureVault(password []byte, fileName string) *SecureVault {
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("could not create secure vault file: %v\n", err)
	}

	salt := make([]byte, 32)
	rand.Read(salt)
	cipherKey, err := scrypt.Key(password, salt, 32768, 8, 1, 32)
	if err != nil {
		log.Fatalf("could not generate cipher key from password and salt: %v\n", err)
	}
	_, secret := crypto.RandomAsymetricKey()
	vault := SecureVault{
		SecretKey:  secret,
		Nodes:      make([]RegisteredNode, 0),
		WalletKeys: make([]WalletKey, 0),
		file:       file,
		cipher:     crypto.CipherFromKey(cipherKey),
	}
	if n, err := file.Write(salt); n != len(salt) || err != nil {
		log.Fatalf("could not write salto to secure vault file: %v\n", err)
	}
	sealed := vault.cipher.Seal(secret[:])
	if n, err := file.Write(append([]byte{byte(len(sealed))}, sealed...)); n != len(sealed)+1 || err != nil {
		log.Fatalf("could not write to secure vault file: %v\n", err)
	}
	return &vault
}
