package main

import (
	"fmt"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/crypto/dh"
)

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
