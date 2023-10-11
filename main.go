package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/freehandle/breeze/consensus/poa"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

var config = poa.SingleAuthorityConfig{
	IncomingPort:     5005,
	OutgoingPort:     5006,
	BlockInterval:    time.Second,
	ValidateIncoming: socket.AcceptAllConnections,
	ValidateOutgoing: socket.AcceptAllConnections,
	WalletFilePath:   "", // memory
	KeepBlocks:       50,
}

var pkHex = "f622f274b13993e3f1824a30ef0f7e57f0c35a4fbdc38e54e37916ef06a64a797eb7aa3582b216bba42d45e91e0a560508478f5b55228439b42733945fd5c2f5"

func main() {
	bytes, _ := hex.DecodeString(pkHex)
	var pk crypto.PrivateKey
	copy(pk[:], bytes)
	tokenBytes, _ := hex.DecodeString(pkHex[64:])
	var token crypto.Token
	copy(token[:], tokenBytes)
	if !pk.PublicKey().Equal(token) {
		log.Fatalf("invalid credentials")
	}
	config.Credentials = pk
	err := <-poa.Genesis(config)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("done")
	}
}
