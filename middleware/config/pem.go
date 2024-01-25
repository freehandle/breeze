package config

import (
	"fmt"
	"os"

	"github.com/freehandle/breeze/crypto"
)

func ParseCredentials(path string, token crypto.Token) (crypto.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return crypto.ZeroPrivateKey, fmt.Errorf("could not read credentials file: %v", err)
	}
	pk, err := crypto.ParsePEMPrivateKey(data)
	if err != nil {
		return crypto.ZeroPrivateKey, fmt.Errorf("could not parse credentials file: %v", err)
	}
	if !pk.PublicKey().Equal(token) {
		return crypto.ZeroPrivateKey, fmt.Errorf("credentials file does not match token: %v instead of %v", pk.PublicKey(), token)
	}
	return pk, nil
}
