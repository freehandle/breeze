package social

import (
	"errors"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

func NewBlockSocial(epoch uint64) []byte {
	bytes := []byte{messages.MsgNewBlock}
	util.PutUint64(epoch, &bytes)
	return bytes
}

func ParseNewBlockSocial(data []byte) (uint64, error) {
	if len(data) < 9 {
		return 0, errors.New("ParseNewBlockSocial: data too short")
	}
	epoch, _ := util.ParseUint64(data, 1)
	return epoch, nil
}

func ActionSocial(action []byte) []byte {
	bytes := []byte{messages.MsgAction}
	bytes = append(bytes, action...)
	return bytes
}

func SealBlockSocial(epoch uint64, hash crypto.Hash) []byte {
	bytes := []byte{messages.MsgSeal}
	util.PutUint64(epoch, &bytes)
	util.PutHash(hash, &bytes)
	return bytes
}

func ParseSealBlockSocial(data []byte) (uint64, crypto.Hash, error) {
	if len(data) != 9+crypto.Size {
		return 0, crypto.ZeroHash, errors.New("ParseSealBlockSocial: data too short")
	}
	epoch, _ := util.ParseUint64(data, 1)
	hash, _ := util.ParseHash(data, 9)
	return epoch, hash, nil
}

func CommitBlockSocial(epoch uint64, invalidated []crypto.Hash) []byte {
	bytes := []byte{messages.MsgCommit}
	util.PutUint64(epoch, &bytes)
	util.PutHashArray(invalidated, &bytes)
	return bytes
}

func ParseCommitBlockSocial(data []byte) (uint64, []crypto.Hash, error) {
	if len(data) < 9 {
		return 0, nil, errors.New("ParseCommitBlockSocial: data too short")
	}
	epoch, _ := util.ParseUint64(data, 1)
	invalidated, _ := util.ParseHashArray(data, 9)
	return epoch, invalidated, nil
}

func NewSyncRequest(epoch uint64) []byte {
	bytes := []byte{messages.MsgSyncRequest}
	util.PutUint64(epoch, &bytes)
	return bytes
}
