package chain

import (
	"fmt"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/protocol/state"
	"github.com/freehandle/breeze/util"
)

const (
	BlockActionOffset = 32 + 8 + 8 + 32 + 32 + 8
)

// BLockBuilder is the primitive to mint a new block. It containes the block
// header, the action array and an action validator.
type BlockBuilder struct {
	Header    BlockHeader
	Actions   *ActionArray
	Validator *state.MutatingState
}

// TODO: is this used??
func (b *BlockBuilder) Serialize() []byte {
	bytes := b.Header.Serialize()
	bytes = append(bytes, b.Actions.Serialize()...)
	return bytes
}

// TODO: is this used?
func (b *BlockBuilder) Clone() *BlockBuilder {
	return &BlockBuilder{
		Header:  b.Header,
		Actions: b.Actions.Clone(),
	}
}

// TODO: is this used?
func (b *BlockBuilder) Hash() crypto.Hash {
	hashHead := crypto.Hasher(b.Header.Serialize())
	hashActions := b.Actions.Hash()
	return crypto.Hasher(append(hashHead[:], hashActions[:]...))
}

// Validates new action and appends it to the action array if valid.
func (b *BlockBuilder) Validate(data []byte) bool {
	fmt.Println("Validating action")
	if b.Validator.Validate(data) {
		fmt.Println("ok")
		b.Actions.Append(data)
		return true
	}
	return false
}

// Returns a pointer to a SeledBlock properly signed by the provided credentials.
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

// Appends a provided seal to the blockbuild and returns a pointer to a SealedBlock.
func (b *BlockBuilder) ImprintSeal(seal BlockSeal) *SealedBlock {
	return &SealedBlock{
		Header:  b.Header,
		Actions: b.Actions,
		Seal:    seal,
	}
}

// SealedBlock  is the sealed version of a block. It should be considered final
// and immutable, but not yet incorporated into the block chain. It consists of
// a block header, an action array and a block seal with a valid signature
// against the block creator expressed in the block header.
type SealedBlock struct {
	Header    BlockHeader
	Actions   *ActionArray
	Seal      BlockSeal
	Mutations *state.Mutations
}

// ParseSealedBlock parses a byte array into a SealedBlock. Returns nil if the
// byte array is not a valid SealedBlock.
func ParseSealedBlock(data []byte) *SealedBlock {
	var block SealedBlock
	header, position := parseHeaderBlockHeaderPosition(data, 0)
	if header == nil {
		return nil
	}
	block.Header = *header
	block.Actions, position = ParseAction(data, position)
	var seal *BlockSeal
	seal, position = parseBlockSealPosition(data, position)
	block.Seal = *seal
	if position != len(data) {
		return nil
	}
	return &block
}

// Serialize serializes a SealedBlock to a byte array.
func (s *SealedBlock) Serialize() []byte {
	bytes := s.Header.Serialize()
	bytes = append(bytes, s.Actions.Serialize()...)
	bytes = append(bytes, s.Seal.Serialize()...)
	return bytes
}

// Revalidate checks if the actions of a sealed block remains valid according to
// a more recent state. Hashes of invalidated actions are added to the block
// commit. The block chain shoul ignore those invalidated actions in order to
// advance the state of the chain. Nonetheless the invalidated actions are not
// purged from the action array in the CommitBlock.
// Commit is an individual action of a node and not subject to a committe for
// further validation and consensus formation. Users receiving a commit block
// must decide if the trust the publisher or if theuy should independtly check
// the validity of the commit metadata.
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

// Returns a pointer to a SealedBlock with the same header, action array and
// seal as the CommitBlock.
func (c *CommitBlock) Sealed() *SealedBlock {
	return &SealedBlock{
		Header:  c.Header,
		Actions: c.Actions,
		Seal:    c.Seal,
	}
}

// ParseCommitBlock parses a byte array into a CommitBlock. Returns nil if the
// byte array is not a valid CommitBlock.
func ParseCommitBlock(data []byte) *CommitBlock {
	var block CommitBlock
	header, position := parseHeaderBlockHeaderPosition(data, 0)
	if header == nil {
		return nil
	}
	block.Header = *header
	block.Actions, position = ParseAction(data, position)
	var seal *BlockSeal
	seal, position = parseBlockSealPosition(data, position)
	block.Seal = *seal
	var commit *BlockCommit
	commit, position = parseBlockCommitPosition(data, position)
	block.Commit = commit
	if position != len(data) {
		return nil
	}
	return &block
}

// Serialize serializes a CommitBlock to a byte array without signature.
func (b *CommitBlock) serializeForPublish() []byte {
	bytes := b.Header.Serialize()
	bytes = append(bytes, b.Actions.Serialize()...)
	bytes = append(bytes, b.Seal.Serialize()...)
	bytes = append(bytes, b.Commit.serializeToSign()...)
	return bytes
}

// Serialize serializes a CommitBlock to a byte array.
func (b *CommitBlock) Serialize() []byte {
	bytes := b.serializeForPublish()
	util.PutSignature(b.Commit.PublishSign, &bytes)
	return bytes
}

// Returns byte byte array of only those actions not invalidated by the block
// commit structure.
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
