package chain

import (
	"time"

	"github.com/freehandle/breeze/consensus/bft"
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
	Duplicate      *bft.Duplicate       // Evidence for rule violations on the consesus pool
	Candidate      []*ChecksumStatement // Validator candidate evidence for state checksum
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
	bft.PutDuplicate(b.Duplicate, &bytes)
	util.PutUint16(uint16(len(b.Candidate)), &bytes)
	for _, candidate := range b.Candidate {
		PutChecksumStatement(candidate, &bytes)
	}
	return bytes
}

func ParseBlockHeader(data []byte) *BlockHeader {
	header, position := parseHeaderBlockHeaderPosition(data, 0)
	if position != len(data) {
		return nil
	}
	return header
}

func parseHeaderBlockHeaderPosition(data []byte, position int) (*BlockHeader, int) {
	var block BlockHeader
	block.NetworkHash, position = util.ParseHash(data, position)
	block.Epoch, position = util.ParseUint64(data, position)
	block.CheckPoint, position = util.ParseUint64(data, position)
	block.CheckpointHash, position = util.ParseHash(data, position)
	block.Proposer, position = util.ParseToken(data, position)
	block.ProposedAt, position = util.ParseTime(data, position)
	block.Duplicate, position = bft.ParseDuplicatePosition(data, position)
	count, position := util.ParseUint16(data, position)
	block.Candidate = make([]*ChecksumStatement, count)
	for i := 0; i < int(count); i++ {
		block.Candidate[i], position = ParseChecksumStatementPosition(data, position)
	}
	return &block, position
}

type BlockSeal struct {
	Hash          crypto.Hash
	FeesCollected uint64
	SealSignature crypto.Signature
	Consensus     []*bft.Ballot
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
	util.PutByte(byte(len(b.Consensus)), &bytes)
	for _, ballot := range b.Consensus {
		bft.PutBallot(ballot, &bytes)
	}
	return bytes
}

func ParseBlockSeal(data []byte) *BlockSeal {
	block, position := parseBlockSealPosition(data, 0)
	if position != len(data) {
		return nil
	}
	return block
}

func parseBlockSealPosition(data []byte, position int) (*BlockSeal, int) {
	var block BlockSeal
	block.Hash, position = util.ParseHash(data, position)
	block.FeesCollected, position = util.ParseUint64(data, position)
	block.SealSignature, position = util.ParseSignature(data, position)
	count, position := util.ParseByte(data, position)
	block.Consensus = make([]*bft.Ballot, count)
	for i := 0; i < int(count); i++ {
		block.Consensus[i], position = bft.ParseBallotPosition(data, position)
	}
	return &block, position
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
	block, position := parseBlockCommitPosition(data, 0)
	if position != len(data) {
		return nil
	}
	return block
}

func parseBlockCommitPosition(data []byte, position int) (*BlockCommit, int) {
	var block BlockCommit
	block.Invalidated, position = util.ParseHashArray(data, position)
	block.FeesCollected, position = util.ParseUint64(data, position)
	block.PublishedBy, position = util.ParseToken(data, position)
	block.PublishSign, position = util.ParseSignature(data, position)
	return &block, position
}
