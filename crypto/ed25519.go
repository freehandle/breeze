// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ed25519 implements the Ed25519 signature algorithm. See
// https://ed25519.cr.yp.to/.
//
// These functions are also compatible with the “Ed25519” function defined in
// RFC 8032. However, unlike RFC 8032's formulation, this package's private key
// representation includes a public key suffix to make multiple signing
// operations with the same key more efficient. This package refers to the RFC
// 8032 private key as the “seed”.
//
// This is a interface adaptation of the original file.
package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"

	"github.com/freehandle/breeze/crypto/edwards25519"
)

const AddressSize = 20

type Signature [SignatureSize]byte

var ZeroSignature Signature

func (s Signature) String() string {
	text, _ := s.MarshalText()
	return string(text)
}

func (s Signature) MarshalText() (text []byte, err error) {
	text = make([]byte, 2*SignatureSize)
	hex.Encode(text, s[:])
	return
}

func (s Signature) UnmarshalText(text []byte) error {
	_, err := hex.Decode(s[:], text)
	return err
}

var ZeroToken Token
var ZeroPrivateKey PrivateKey

func RandomAsymetricKey() (Token, PrivateKey) {

	var public [32]byte
	var private PrivateKey

	seed := make([]byte, PublicKeySize)
	rand.Read(seed)
	digest := sha512.Sum512(seed)
	digest[0] &= 248
	digest[31] &= 127
	digest[31] |= 64

	var A edwards25519.ExtendedGroupElement
	var hBytes [32]byte
	copy(hBytes[:], digest[:])
	edwards25519.GeScalarMultBase(&A, &hBytes)
	A.ToBytes(&public)
	copy(private[0:32], seed)
	copy(private[32:], public[:])
	return Token(public), private
}

func PrivateKeyFromSeed(seed [32]byte) PrivateKey {
	var public [32]byte
	var private PrivateKey

	digest := sha512.Sum512(seed[:])
	digest[0] &= 248
	digest[31] &= 127
	digest[31] |= 64

	var A edwards25519.ExtendedGroupElement
	var hBytes [32]byte
	copy(hBytes[:], digest[:])
	edwards25519.GeScalarMultBase(&A, &hBytes)
	A.ToBytes(&public)
	copy(private[0:32], seed[:])
	copy(private[32:], public[:])
	return private
}

func IsValidPrivateKey(data []byte) bool {
	if len(data) != PrivateKeySize {
		return false
	}
	pk := PrivateKeyFromSeed([32]byte(data[0:32]))
	for n := 0; n < PrivateKeySize; n++ {
		if pk[n] != data[n] {
			return false
		}
	}
	return true
}

type PrivateKey [PrivateKeySize]byte

func (p PrivateKey) PublicKey() Token {
	var token Token
	copy(token[:], p[32:])
	return token
}

func (p PrivateKey) Hex() string {
	return hex.EncodeToString(p[:])
}

func (p PrivateKey) Sign(msg []byte) Signature {

	var signature Signature

	h := sha512.New()
	h.Write(p[:32])
	var digest1, messageDigest, hramDigest [64]byte
	var expandedSecretKey [32]byte
	h.Sum(digest1[:0])
	copy(expandedSecretKey[:], digest1[:])
	expandedSecretKey[0] &= 248
	expandedSecretKey[31] &= 63
	expandedSecretKey[31] |= 64

	h.Reset()
	h.Write(digest1[32:])
	h.Write(msg)
	h.Sum(messageDigest[:0])

	var messageDigestReduced [32]byte
	edwards25519.ScReduce(&messageDigestReduced, &messageDigest)
	var R edwards25519.ExtendedGroupElement
	edwards25519.GeScalarMultBase(&R, &messageDigestReduced)

	var encodedR [32]byte
	R.ToBytes(&encodedR)

	h.Reset()
	h.Write(encodedR[:])
	h.Write(p[32:])
	h.Write(msg)
	h.Sum(hramDigest[:0])
	var hramDigestReduced [32]byte
	edwards25519.ScReduce(&hramDigestReduced, &hramDigest)

	var s [32]byte
	edwards25519.ScMulAdd(&s, &hramDigestReduced, &expandedSecretKey, &messageDigestReduced)

	copy(signature[:32], encodedR[:])
	copy(signature[32:], s[:])
	return signature
}

type Token [TokenSize]byte

func (t Token) String() string {
	text, _ := t.MarshalText()
	return string(text)
}

func (t Token) Hex() string {
	return hex.EncodeToString(t[:])
}

func (t Token) MarshalText() (text []byte, err error) {
	text = make([]byte, 2*TokenSize)
	hex.Encode(text, t[:])
	return
}

func (t Token) UnmarshalText(text []byte) error {
	_, err := hex.Decode(t[:], text)
	return err
}

func (t Token) Equals(bytes []byte) bool {
	if len(bytes) != TokenSize {
		return false
	}
	for n := 0; n < TokenSize; n++ {
		if bytes[n] != t[n] {
			return false
		}
	}
	return true
}

func TokenFromString(s string) Token {
	var token Token
	if bytes, err := hex.DecodeString(s); err == nil {
		copy(token[:], bytes)
	}
	return token
}

func (t Token) Larger(another Token) bool {
	for n := 0; n < TokenSize; n++ {
		if t[n] > another[n] {
			return true
		} else if t[n] < another[n] {
			return false
		}
	}
	return false
}

func (t Token) Equal(another Token) bool {
	return t == another
}

func (t Token) Verify(msg []byte, signature Signature) bool {
	if signature[63]&224 != 0 {
		return false
	}

	var A edwards25519.ExtendedGroupElement
	publicKey := [32]byte(t)
	if !A.FromBytes(&publicKey) {
		return false
	}
	edwards25519.FeNeg(&A.X, &A.X)
	edwards25519.FeNeg(&A.T, &A.T)

	h := sha512.New()
	h.Write(signature[:32])
	h.Write(publicKey[:])
	h.Write(msg)
	var digest [64]byte
	h.Sum(digest[:0])

	var hReduced [32]byte
	edwards25519.ScReduce(&hReduced, &digest)

	var R edwards25519.ProjectiveGroupElement
	var s [32]byte
	copy(s[:], signature[32:])

	// https://tools.ietf.org/html/rfc8032#section-5.1.7 requires that s be in
	// the range [0, order) in order to prevent signature malleability.
	if !edwards25519.ScMinimal(&s) {
		return false
	}

	edwards25519.GeDoubleScalarMultVartime(&R, &hReduced, &A, &s)

	var checkR [32]byte
	R.ToBytes(&checkR)
	return bytes.Equal(signature[:32], checkR[:])

}

type Address [AddressSize]byte

func (t Token) Address() Address {
	var address Address
	hash := HashToken(t)
	copy(address[:], hash[0:20])
	return address
}

func (t Token) IsLike(a Address) bool {
	hash := HashToken(t)
	for n := 0; n < AddressSize; n++ {
		if hash[n] != a[n] {
			return false
		}
	}
	return true
}

func (a Address) String() string {
	text, _ := a.MarshalText()
	return string(text)
}

func (a Address) Hex() string {
	return hex.EncodeToString(a[:])
}

func (a Address) MarshalText() (text []byte, err error) {
	text = make([]byte, 2*AddressSize)
	hex.Encode(text, a[:])
	return
}
