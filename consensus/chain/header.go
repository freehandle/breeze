package chain

import (
	"time"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

// BlockHeader communicates the intention of a validator to mint a new block.
// It contains the network hash, the epoch, the checkpoint <= epoch - 1 against
// which the validator will check the validity of proposed actions, the hash of
// the block associated to that checkpoint, the validator's token, the UTC time
// as perceived by the validator at the submission of the header, optional
// evidence of previous infringiments from other nodes of the swell protocol,
// and optional checksum statments as received by the validator from cadidate
// validating nodes for the next checksum window.
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

// Clone returns a copy of the block header without duplicates and checksum
// statements.
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

// Serialize serializes a block header to a byte slice.
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

// ParseBlockHeader parses a byte slice to a block header. Return nil if the
// byte slice does not contain a valid block header.
func ParseBlockHeader(data []byte) *BlockHeader {
	header, position := parseHeaderBlockHeaderPosition(data, 0)
	if position != len(data) {
		return nil
	}
	return header
}

// ParseBlockHeaderPosition parses a block header in the middle of a byte slice
// and returns the parsed block header and the position at the end of the header
// bytes.
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

// Seal for a proposed block. It contains the hash of the proposed block, the
// fees collected by the validator, the signature of the proposer of the block,
// and the consensus ballots for the block. Consensus information is gathered
// individually by each validator by the bft rules. Anyone in possession of
// this can attest to the validity of the bft consensus algorithm.
type BlockSeal struct {
	Hash          crypto.Hash
	FeesCollected uint64
	SealSignature crypto.Signature
	Consensus     []*bft.Ballot
}

// Clone returns a copy of the block seal without consensus information.
func (b BlockSeal) Clone() BlockSeal {
	return BlockSeal{
		Hash:          b.Hash,
		FeesCollected: b.FeesCollected,
		SealSignature: b.SealSignature,
	}
}

// Serialize serializes a block seal to a byte slice.
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

// ParseBlockSeal parses a byte slice to a block seal. Return nil if the byte
// slice does not contain a valid block seal.
func ParseBlockSeal(data []byte) *BlockSeal {
	block, position := parseBlockSealPosition(data, 0)
	if position != len(data) {
		return nil
	}
	return block
}

// ParseBlockSealPosition parses a block seal in the middle of a byte slice and
// returns the parsed block seal and the position at the end of the seal bytes.
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

// BlockCommit is the final structure of a block. Every validating node must
// build a block commit structure for every consensual sealed block it receives.
// Blocks are commit in sequence. Sealed block for epoch t can only be committed
// if block for epoch t - 1 has been committed. At the commit process a node
// must revalidate all the proposed actions against the state at the previous
// block. Hashes of invalidated actions are groupéd in the block commit. The
// ammount of fees collected by the proposer of the block is also recalculated
// to exclude the fees of invalidated or previously incorporated actions.
// Every node must publish its own perception of the block commit structure.
// If the consensys algorithm is working, every node will have the same
// perception of reality. The swell protocol does not anticipate penalties for
// faulty commits.
type BlockCommit struct {
	Invalidated   []crypto.Hash
	FeesCollected uint64
	PublishedBy   crypto.Token
	PublishSign   crypto.Signature
}

// Serialize serializes a block commit to a byte slice without signature.
func (b BlockCommit) serializeToSign() []byte {
	bytes := make([]byte, 0)
	util.PutHashArray(b.Invalidated, &bytes)
	util.PutUint64(b.FeesCollected, &bytes)
	util.PutToken(b.PublishedBy, &bytes)
	return bytes
}

// Serialize serializes a block commit to a byte slice.
func (b BlockCommit) Serialize() []byte {
	bytes := b.serializeToSign()
	util.PutSignature(b.PublishSign, &bytes)
	return bytes
}

// ParseBlockCommit parses a byte slice to a block commit. Return nil if the
// byte slice does not contain a valid block commit.
func ParseBlockCommit(data []byte) *BlockCommit {
	block, position := parseBlockCommitPosition(data, 0)
	if position != len(data) {
		return nil
	}
	return block
}

// ParseBlockCommitPosition parses a block commit in the middle of a byte slice
// and returns the parsed block commit and the position at the end of the commit
// bytes.
func parseBlockCommitPosition(data []byte, position int) (*BlockCommit, int) {
	var block BlockCommit
	block.Invalidated, position = util.ParseHashArray(data, position)
	block.FeesCollected, position = util.ParseUint64(data, position)
	block.PublishedBy, position = util.ParseToken(data, position)
	block.PublishSign, position = util.ParseSignature(data, position)
	return &block, position
}
