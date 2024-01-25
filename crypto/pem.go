package crypto

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
)

var ErrPrivateKeyParse = errors.New("could not parse private key")
var ErrPublicKeyParse = errors.New("could not parse public key")

var oidKeyEd25519 = asn1.ObjectIdentifier{1, 3, 101, 112}

// pkcs8 reflects an ASN.1, PKCS #8 PrivateKey.
type pkcs8 struct {
	Version    int
	Algo       pkix.AlgorithmIdentifier
	PrivateKey []byte
}

type publicKeyInfo struct {
	Raw       asn1.RawContent
	Algorithm pkix.AlgorithmIdentifier
	PublicKey asn1.BitString
}

func ParsePEMPrivateKey(data []byte) (PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return ZeroPrivateKey, ErrPrivateKeyParse
	}
	var key pkcs8
	if _, err := asn1.Unmarshal(block.Bytes, &key); err != nil {
		return ZeroPrivateKey, ErrPrivateKeyParse
	}
	if !key.Algo.Algorithm.Equal(oidKeyEd25519) {
		return ZeroPrivateKey, ErrPrivateKeyParse
	}
	var bytes []byte
	if _, err := asn1.Unmarshal(key.PrivateKey, &bytes); err != nil {
		return ZeroPrivateKey, ErrPrivateKeyParse
	}
	var seed [32]byte
	copy(seed[:], bytes)
	return PrivateKeyFromSeed(seed), nil
}

func ParsePEMPublicKey(data []byte) (Token, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" {
		return ZeroToken, ErrPublicKeyParse
	}
	var pki publicKeyInfo
	if _, err := asn1.Unmarshal(block.Bytes, &pki); err != nil {
		fmt.Println(err)
		return ZeroToken, ErrPublicKeyParse
	}
	if !pki.Algorithm.Algorithm.Equal(oidKeyEd25519) {
		fmt.Println(pki.Algorithm.Algorithm)
		return ZeroToken, ErrPublicKeyParse
	}
	var token Token
	copy(token[:], pki.PublicKey.RightAlign())
	return token, nil
}
