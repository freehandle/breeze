package chain

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/protocol/state"
	"github.com/freehandle/breeze/util"
)

const (
	BlockActionOffset = 32 + 8 + 8 + 32 + 32 + 8
)

type BlockBuilder struct {
	Header    BlockHeader
	Actions   *ActionArray
	Validator *state.MutatingState
}

func (b *BlockBuilder) Serialize() []byte {
	bytes := b.Header.Serialize()
	bytes = append(bytes, b.Actions.Serialize()...)
	return bytes
}

func (b *BlockBuilder) Clone() *BlockBuilder {
	return &BlockBuilder{
		Header:  b.Header,
		Actions: b.Actions.Clone(),
	}
}

func (b *BlockBuilder) Hash() crypto.Hash {
	hashHead := crypto.Hasher(b.Header.Serialize())
	hashActions := b.Actions.Hash()
	return crypto.Hasher(append(hashHead[:], hashActions[:]...))
}

func (b *BlockBuilder) Validate(data []byte) bool {
	if b.Validator.Validate(data) {
		b.Actions.Append(data)
		return true
	}
	return false
}

func (b *BlockBuilder) Seal(credentials crypto.PrivateKey) *SealedBlock {
	hash := b.Hash()
	signature := credentials.Sign(hash[:])
	return &SealedBlock{
		Header:  b.Header,
		Actions: b.Actions,
		Seal: BlockSeal{
			Hash:          hash,
			FeesCollected: b.Validator.FeesCollected,
			SealSignature: signature,
		},
		Mutations: b.Validator.Mutations(),
	}
}

func (b *BlockBuilder) ImprintSeal(seal BlockSeal) *SealedBlock {
	return &SealedBlock{
		Header:  b.Header,
		Actions: b.Actions,
		Seal:    seal,
	}
}

type SealedBlock struct {
	Header    BlockHeader
	Actions   *ActionArray
	Seal      BlockSeal
	Mutations *state.Mutations
}

func ParseSealedBlock(data []byte) *SealedBlock {
	var block SealedBlock
	position := 0
	block.Header.NetworkHash, position = util.ParseHash(data, position)
	block.Header.Epoch, position = util.ParseUint64(data, position)
	block.Header.CheckPoint, position = util.ParseUint64(data, position)
	block.Header.CheckpointHash, position = util.ParseHash(data, position)
	block.Header.Proposer, position = util.ParseToken(data, position)
	block.Header.ProposedAt, position = util.ParseTime(data, position)
	block.Actions, position = ParseAction(data, position)
	block.Seal.Hash, position = util.ParseHash(data, position)
	block.Seal.FeesCollected, position = util.ParseUint64(data, position)
	block.Seal.SealSignature, position = util.ParseSignature(data, position)
	if position != len(data) {
		return nil
	}
	return &block
}

func (s *SealedBlock) Serialize() []byte {
	bytes := s.Header.Serialize()
	bytes = append(bytes, s.Actions.Serialize()...)
	bytes = append(bytes, s.Seal.Serialize()...)
	return bytes
}

func (c *SealedBlock) Revalidate(validator *state.MutatingState, publish crypto.PrivateKey) *CommitBlock {
	invalidated := make([]crypto.Hash, 0)
	feesCollected := c.Seal.FeesCollected
	if validator != nil {
		for n := 0; n < c.Actions.Len(); n++ {
			action := c.Actions.Get(n)
			if !validator.Validate(action) {
				actionFee := actions.GetFeeFromBytes(action)
				if feesCollected > actionFee {
					feesCollected -= actionFee
				}
				invalidated = append(invalidated, crypto.Hasher(action))
			}
		}
	}
	block := &CommitBlock{
		Header:  c.Header,
		Actions: c.Actions,
		Seal:    c.Seal,
		Commit: &BlockCommit{
			Invalidated:   invalidated,
			FeesCollected: feesCollected,
			PublishedBy:   publish.PublicKey(),
		},
		mutations: validator.Mutations(),
	}
	bytes := block.serializeForPublish()
	block.Commit.PublishSign = publish.Sign(bytes)
	return block
}

type CommitBlock struct {
	Header    BlockHeader
	Actions   *ActionArray
	Seal      BlockSeal
	Commit    *BlockCommit
	mutations *state.Mutations
}

func ParseCommitBlock(data []byte) *CommitBlock {
	var block CommitBlock
	position := 0
	block.Header.NetworkHash, position = util.ParseHash(data, position)
	block.Header.Epoch, position = util.ParseUint64(data, position)
	block.Header.CheckPoint, position = util.ParseUint64(data, position)
	block.Header.CheckpointHash, position = util.ParseHash(data, position)
	block.Header.Proposer, position = util.ParseToken(data, position)
	block.Header.ProposedAt, position = util.ParseTime(data, position)
	block.Actions, position = ParseAction(data, position)
	block.Seal.Hash, position = util.ParseHash(data, position)
	block.Seal.FeesCollected, position = util.ParseUint64(data, position)
	block.Seal.SealSignature, position = util.ParseSignature(data, position)
	block.Commit = &BlockCommit{}
	block.Commit.Invalidated, position = util.ParseHashArray(data, position)
	block.Commit.FeesCollected, position = util.ParseUint64(data, position)
	block.Commit.PublishedBy, position = util.ParseToken(data, position)
	block.Commit.PublishSign, position = util.ParseSignature(data, position)
	if position != len(data) {
		return nil
	}
	return &block
}

func (b *CommitBlock) serializeForPublish() []byte {
	bytes := b.Header.Serialize()
	bytes = append(bytes, b.Actions.Serialize()...)
	bytes = append(bytes, b.Seal.Serialize()...)
	bytes = append(bytes, b.Commit.serializeToSign()...)
	return bytes
}

func (b *CommitBlock) Serialize() []byte {
	bytes := b.serializeForPublish()
	util.PutSignature(b.Commit.PublishSign, &bytes)
	return bytes
}

func (b *CommitBlock) GetValidActions() [][]byte {
	actions := make([][]byte, 0)
	for n := 0; n < b.Actions.Len(); n++ {
		action := b.Actions.Get(n)
		hash := crypto.Hasher(action)
		valid := true
		for _, invalid := range b.Commit.Invalidated {
			if hash.Equal(invalid) {
				valid = false
				break
			}
		}
		if valid {
			actions = append(actions, action)
		}
	}
	return actions
}
