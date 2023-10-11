package chain

import (
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

type BlockHeader struct {
	NetworkHash    crypto.Hash
	Epoch          uint64
	CheckPoint     uint64
	CheckpointHash crypto.Hash
	Proposer       crypto.Token
	ProposedAt     time.Time
}

func (b BlockHeader) Clone() BlockHeader {
	return BlockHeader{
		NetworkHash:    b.NetworkHash,
		Epoch:          b.Epoch,
		CheckPoint:     b.CheckPoint,
		CheckpointHash: b.CheckpointHash,
		Proposer:       b.Proposer,
		ProposedAt:     b.ProposedAt,
	}
}

func (b BlockHeader) Serialize() []byte {
	bytes := make([]byte, 0)
	util.PutHash(b.NetworkHash, &bytes)
	util.PutUint64(b.Epoch, &bytes)
	util.PutUint64(b.CheckPoint, &bytes)
	util.PutHash(b.CheckpointHash, &bytes)
	util.PutToken(b.Proposer, &bytes)
	util.PutTime(b.ProposedAt, &bytes)
	return bytes
}

func ParseBlockHeader(data []byte) *BlockHeader {
	position := 0
	var block BlockHeader
	block.NetworkHash, position = util.ParseHash(data, position)
	block.Epoch, position = util.ParseUint64(data, position)
	block.CheckPoint, position = util.ParseUint64(data, position)
	block.CheckpointHash, position = util.ParseHash(data, position)
	block.Proposer, position = util.ParseToken(data, position)
	block.ProposedAt, position = util.ParseTime(data, position)
	if position != len(data) {
		return nil
	}
	return &block
}

type BlockSeal struct {
	Hash          crypto.Hash
	FeesCollected uint64
	SealSignature crypto.Signature
}

func (b BlockSeal) Clone() BlockSeal {
	return BlockSeal{
		Hash:          b.Hash,
		SealSignature: b.SealSignature,
	}
}

func (b BlockSeal) Serialize() []byte {
	bytes := make([]byte, 0)
	util.PutHash(b.Hash, &bytes)
	util.PutUint64(b.FeesCollected, &bytes)
	util.PutSignature(b.SealSignature, &bytes)
	return bytes
}

func ParseBlockSeal(data []byte) *BlockSeal {
	position := 0
	var block BlockSeal
	block.Hash, position = util.ParseHash(data, position)
	block.FeesCollected, position = util.ParseUint64(data, position)
	block.SealSignature, position = util.ParseSignature(data, position)
	if position != len(data) {
		return nil
	}
	return &block
}

type BlockCommit struct {
	Invalidated   []crypto.Hash
	FeesCollected uint64
	PublishedBy   crypto.Token
	PublishSign   crypto.Signature
}

func (b BlockCommit) serializeToSign() []byte {
	bytes := make([]byte, 0)
	util.PutHashArray(b.Invalidated, &bytes)
	util.PutUint64(b.FeesCollected, &bytes)
	util.PutToken(b.PublishedBy, &bytes)
	return bytes
}

func (b BlockCommit) Serialize() []byte {
	bytes := b.serializeToSign()
	util.PutSignature(b.PublishSign, &bytes)
	return bytes
}

func ParseBlockCommit(data []byte) *BlockCommit {
	position := 0
	var block BlockCommit
	block.Invalidated, position = util.ParseHashArray(data, position)
	block.FeesCollected, position = util.ParseUint64(data, position)
	block.PublishedBy, position = util.ParseToken(data, position)
	block.PublishSign, position = util.ParseSignature(data, position)
	if position != len(data) {
		return nil
	}
	return &block
}
