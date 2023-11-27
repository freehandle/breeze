package swell

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

const CandidateMsg byte = 200

type ValidatorCandidate struct {
	Epoch     uint64
	Token     crypto.Token
	Proof     crypto.Hash
	Signature crypto.Signature
}

func AttestCandidate(data []byte, checksum crypto.Hash, checksumEpoch uint64) bool {
	var epoch uint64
	var token crypto.Token
	var proof crypto.Hash
	var signature crypto.Signature

	if len(data) == 0 || data[0] != CandidateMsg {
		return false
	}
	position := 1
	epoch, position = util.ParseUint64(data, position)
	if epoch != checksumEpoch {
		return false
	}
	token, position = util.ParseToken(data, position)
	proof, position = util.ParseHash(data, position)
	if !proof.Equal(crypto.Hasher(append(token[:], checksum[:]...))) {
		return false
	}
	signature, _ = util.ParseSignature(data, position)
	return token.Verify(data[:position], signature)
}

func Candidate(credentials crypto.PrivateKey, epoch uint64, checksum crypto.Hash) []byte {
	bytes := []byte{CandidateMsg}
	token := credentials.PublicKey()
	util.PutUint64(epoch, &bytes)
	util.PutToken(token, &bytes)
	util.PutHash(crypto.Hasher(append(token[:], checksum[:]...)), &bytes)
	signature := credentials.Sign(bytes)
	util.PutSignature(signature, &bytes)
	return bytes
}
