package social

import (
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

const (
	Unsealed byte = iota
	Sealed
	Committed
)

// Ancestor is a signed hash associateds to a protocol code. A list of ancestors
// is used to provide pedigree for a block.
// Signatures for both the seal and hash are required.
type Ancestor struct {
	ProtocolCode    uint32
	Publisher       crypto.Token
	CommitHash      crypto.Hash
	CommitSignature crypto.Signature
}

func PutPedigree(pedigree []Ancestor, data *[]byte) {
	if len(pedigree) > 0xFFFF {
		slog.Error("SocialBlock: pedigree too large")
		pedigree = pedigree[:0xFFFF]
	}
	util.PutUint16(uint16(len(pedigree)), data)
	for _, hash := range pedigree {
		util.PutUint32(hash.ProtocolCode, data)
		util.PutToken(hash.Publisher, data)
		util.PutHash(hash.CommitHash, data)
		util.PutSignature(hash.CommitSignature, data)
	}
}

func ParsePedigree(data []byte, position int) ([]Ancestor, int) {
	var size uint16
	size, position = util.ParseUint16(data, position)
	if size == 0 {
		return nil, position
	}
	pedigree := make([]Ancestor, size)
	for n := uint16(0); n < size; n++ {
		pedigree[n].ProtocolCode, position = util.ParseUint32(data, position)
		pedigree[n].Publisher, position = util.ParseToken(data, position)
		pedigree[n].CommitHash, position = util.ParseHash(data, position)
		pedigree[n].CommitSignature, position = util.ParseSignature(data, position)
	}
	if position > len(data) {
		return nil, len(data) + 1
	}
	return pedigree, position
}

type SocialBlock struct {
	ProtocolCode    uint32
	Epoch           uint64
	Checkpoint      uint64
	BreezeBlock     crypto.Hash
	Pedigree        []Ancestor
	Actions         *chain.ActionArray
	Invalidated     []crypto.Hash
	Publisher       crypto.Token // publisher and validator for the block
	SealHash        crypto.Hash
	SealSignature   crypto.Signature // signature by publisher of SealHash
	CommitHash      crypto.Hash
	CommitSignature crypto.Signature // signature by publisher of CommitHash
	Status          byte
}

func (s *SocialBlock) Header() []byte {
	data := make([]byte, 0)
	util.PutUint32(s.ProtocolCode, &data)
	util.PutUint64(s.Epoch, &data)
	util.PutUint64(s.Checkpoint, &data)
	util.PutHash(s.BreezeBlock, &data)
	PutPedigree(s.Pedigree, &data)
	util.PutToken(s.Publisher, &data)
	return data
}

func (s *SocialBlock) Commit() *SocialBlockCommit {
	if s.Status != Committed {
		return nil
	}
	return &SocialBlockCommit{
		ProtocolCode:    s.ProtocolCode,
		Epoch:           s.Epoch,
		Publisher:       s.Publisher,
		SealHash:        s.SealHash,
		Invalidated:     s.Invalidated,
		CommitHash:      s.CommitHash,
		CommitSignature: s.CommitSignature,
	}
}

type SocialBlockCommit struct {
	ProtocolCode    uint32
	Epoch           uint64
	Publisher       crypto.Token
	SealHash        crypto.Hash
	Invalidated     []crypto.Hash
	CommitHash      crypto.Hash
	CommitSignature crypto.Signature
}

func (s *SocialBlockCommit) Serialize() []byte {
	bytes := make([]byte, 0)
	util.PutUint32(s.ProtocolCode, &bytes)
	util.PutUint64(s.Epoch, &bytes)
	util.PutToken(s.Publisher, &bytes)
	util.PutHash(s.SealHash, &bytes)
	util.PutHashArray(s.Invalidated, &bytes)
	util.PutHash(s.CommitHash, &bytes)
	util.PutSignature(s.CommitSignature, &bytes)
	return bytes
}

func ParseSocialBlockCommit(data []byte) *SocialBlockCommit {
	commit := SocialBlockCommit{}
	position := 0
	commit.ProtocolCode, position = util.ParseUint32(data, position)
	commit.Epoch, position = util.ParseUint64(data, position)
	commit.Publisher, position = util.ParseToken(data, position)
	commit.SealHash, position = util.ParseHash(data, position)
	commit.Invalidated, position = util.ParseHashArray(data, position)
	commit.CommitHash, position = util.ParseHash(data, position)
	commit.CommitSignature, position = util.ParseSignature(data, position)
	if position > len(data) {
		return nil
	}
	if !commit.Publisher.Verify(commit.CommitHash[:], commit.CommitSignature) {
		return nil
	}
	return &commit
}

type SocialBlockBuilder[M Merger[M], B Blocker[M]] struct {
	Block     *SocialBlock
	Validator B
}

func (s *SocialBlockBuilder[M, B]) Validate(data []byte) bool {
	if s.Block.Status != Unsealed {
		slog.Error("SocialBlock: Validate called on sealed block")
		return false
	}
	if len(data) == 0 {
		return false
	}
	if s.Validator.Validate(data) {
		s.Block.Actions.Append(data)
		return true
	}
	return false
}

func (s *SocialBlock) Seal(credentials crypto.PrivateKey) {
	bytes := s.serializeToSealHash()
	s.SealHash = crypto.Hasher(bytes)
	s.SealSignature = credentials.Sign(s.SealHash[:])
	s.Status = Sealed
}

func (s *SocialBlockBuilder[M, B]) Revalidate(invalid []crypto.Hash, credentials crypto.PrivateKey) {
	invalidSet := util.SetFromSlice(invalid)
	if s.Block.Status == Unsealed {
		slog.Error("SocialBlock: Revalidate called on unsealed block")
		// seal block nonetheless
		s.Block.Seal(credentials)
	} else if s.Block.Status == Committed {
		slog.Error("SocialBlock: Revalidate called on committed block")
		return
	}
	s.Block.Invalidated = make([]crypto.Hash, 0)
	for n := 0; n < s.Block.Actions.Len(); n++ {
		action := s.Block.Actions.Get(n)
		hash := crypto.Hasher(action)
		if _, ok := invalidSet[hash]; !ok {
			s.Block.Invalidated = append(s.Block.Invalidated, hash)
		} else if !s.Validate(action) {
			s.Block.Invalidated = append(s.Block.Invalidated, hash)
		}
	}
	bytes := s.Block.serializeToCommitHash()
	s.Block.CommitHash = crypto.Hasher(bytes)
	s.Block.CommitSignature = credentials.Sign(s.Block.CommitHash[:])
	s.Block.Status = Committed
}

func (s *SocialBlockBuilder[M, B]) Mutations() M {
	return s.Validator.Mutations()
}

func (s *SocialBlock) serializeToSealHash() []byte {
	data := make([]byte, 0)
	util.PutUint32(s.ProtocolCode, &data)
	util.PutUint64(s.Epoch, &data)
	util.PutUint64(s.Checkpoint, &data)
	util.PutHash(s.BreezeBlock, &data)
	PutPedigree(s.Pedigree, &data)
	util.PutToken(s.Publisher, &data)
	util.PutLargeByteArray(s.Actions.Serialize(), &data)
	return data
}

func (s *SocialBlock) serializeToCommitHash() []byte {
	data := s.serializeToSealHash()
	util.PutHashArray(s.Invalidated, &data)
	return data
}

func (s *SocialBlock) Serialize() []byte {
	bytes := s.serializeToCommitHash()
	util.PutSignature(s.SealSignature, &bytes)
	util.PutSignature(s.CommitSignature, &bytes)
	return bytes
}

func ParseSocialBlock(data []byte) *SocialBlock {
	block := SocialBlock{}
	position := 0
	block.ProtocolCode, position = util.ParseUint32(data, position)
	block.Epoch, position = util.ParseUint64(data, position)
	block.Checkpoint, position = util.ParseUint64(data, position)
	block.BreezeBlock, position = util.ParseHash(data, position)
	block.Pedigree, position = ParsePedigree(data, position)
	block.Actions, position = chain.ParseAction(data, position)
	block.SealHash = crypto.Hasher(data[:position])
	block.Invalidated, position = util.ParseHashArray(data, position)
	block.CommitHash = crypto.Hasher(data[:position])
	block.Publisher, position = util.ParseToken(data, position)
	block.SealSignature, position = util.ParseSignature(data, position)
	block.CommitSignature, position = util.ParseSignature(data, position)
	if position > len(data) {
		return nil
	}
	if block.SealSignature == crypto.ZeroSignature {
		block.Status = Unsealed
	} else if block.CommitSignature == crypto.ZeroSignature {
		block.Status = Sealed
		if !block.Publisher.Verify(block.SealHash[:], block.SealSignature) {
			return nil
		}
	} else {
		block.Status = Committed
		if !block.Publisher.Verify(block.CommitHash[:], block.CommitSignature) {
			return nil
		}
	}
	return &block
}
