package main

import (
	"fmt"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/crypto/dh"
	"github.com/freehandle/breeze/middleware/config"
)

// Echoconfig is the configuration for an echo service
type EchoConfig struct {
	// Token of the service... credentials will be provided by diffie-hellman
	Token string
	// The address of the service (IP or domain name)
	Address string
	// Port for admin connections
	AdminPort int
	// WalletPath should be empty for memory based wallet store
	// OR should be a path to a valid folder with appropriate permissions
	WalletPath string
	// LogPath should be empty for standard logging
	// OR should be a path to a valid folder with appropriate permissions
	LogPath string
	// Breeze network configuration
	Breeze config.BreezeConfig

	TrustedNode []config.Peer
}

func main() {
	remotePk, remote := dh.NewEphemeralKey()
	ephPk, eph := dh.NewEphemeralKey()
	_, pk := crypto.RandomAsymetricKey()
	cipher := dh.ConsensusCipher(ephPk, remote)
	remotePKCipher := cipher.Seal(pk[:])

	cipher2 := dh.ConsensusCipher(remotePk, eph)
	data, err := cipher2.Open(remotePKCipher)
	if err != nil {
		panic(err)
	}
	var cpk crypto.PrivateKey
	copy(cpk[:], data)

	fmt.Println(pk, remotePk)

	fmt.Println(crypto.IsValidPrivateKey(remotePk[:]))
	fmt.Println(crypto.IsValidPrivateKey(cpk[:]))
}
