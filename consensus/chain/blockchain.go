/*
	chain provides blockchain interface

Rules for the chain mechanism:

 1. blocks are proposed for a certain epoch and against a certain checkpoint
    prior to that epoch.
 2. the block associated to a checkpoint must be sealed, otherwise it is not a
    valid checkpoint. sealed blocks cannot append new actions. They are not
    considerer final because certain actions can be removed by the commit phase.
 2. actions for the block are temporarily validated against the state derived
    at the checkpoint epoch.
 3. blocks are sealed, a hash is calculated, and the hash is signed by the
    publisher of the block. the commit phase is done by every node
 4. blocks are commited with all transactions validated with the checkpoint of
    the epoch immediately before the block epoch. Actions that were approved as
    validated by the original checkpoint are marked as invalidated by the commit
    instruction.
*/
package chain

import (
	"errors"
	"sync"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/protocol/state"
)

const KeepLastNBlocks = 100

type Checksum struct {
	Epoch uint64
	State *state.State
	Hash  crypto.Hash
}

// Chain is a non-disputed block interface... one block proposed for each
// epoch, every block is sealed before the proposal of a new block.
// Final commit of blocks can be delayed and the chain might be asked to
// rollover to any epoch after the last commit epoch. disaster recovery,
// that means, the rollover before last commit epoch is not anticipated on the
// structure and must be implemented separatedly.
type Chain struct {
	mu              sync.Mutex
	NetworkHash     crypto.Hash
	Incorporated    *IncorporatedActions
	Credentials     crypto.PrivateKey
	LastCommitEpoch uint64
	LastCommitHash  crypto.Hash
	CommitState     *state.State
	LiveBlock       *BlockBuilder
	UnsealedBlocks  []*BlockBuilder
	SealedBlocks    []*SealedBlock
	RecentBlocks    []*CommitBlock
	Cloning         bool
	Checksum        *Checksum
}

func NewChainFromGenesisState(credentials crypto.PrivateKey, walletPath string) *Chain {
	genesis := state.NewGenesisStateWithToken(credentials.PublicKey(), walletPath)
	if genesis == nil {
		return nil
	}
	return &Chain{
		mu:              sync.Mutex{},
		NetworkHash:     crypto.HashToken(credentials.PublicKey()),
		Incorporated:    NewIncorporatedActions(0),
		Credentials:     credentials,
		LastCommitEpoch: 0,
		LastCommitHash:  crypto.HashToken(credentials.PublicKey()),
		CommitState:     genesis,
		UnsealedBlocks:  make([]*BlockBuilder, 0),
		SealedBlocks:    make([]*SealedBlock, 0),
		RecentBlocks:    make([]*CommitBlock, 0),
		Checksum: &Checksum{
			Epoch: 0,
			State: genesis,
			Hash:  crypto.HashToken(credentials.PublicKey()),
		},
	}
}

func (c *Chain) NextBlock(epoch uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Incorporated.MoveForward()
	c.LiveBlock = &BlockBuilder{
		Header: BlockHeader{
			NetworkHash:    c.NetworkHash,
			Epoch:          epoch,
			CheckPoint:     c.LastCommitEpoch,
			CheckpointHash: c.LastCommitHash,
			Proposer:       c.Credentials.PublicKey(),
			ProposedAt:     time.Now(),
		},
		Actions:   NewActionArray(),
		Validator: c.CommitState.Validator(state.NewMutations(epoch), epoch),
	}
}

func NewChainFromChecksumState(c *Checksum, credentials crypto.PrivateKey, lastBlockHash crypto.Hash) *Chain {
	return &Chain{
		mu:              sync.Mutex{},
		Incorporated:    NewIncorporatedActions(c.Epoch),
		Credentials:     credentials,
		LastCommitEpoch: c.Epoch,
		LastCommitHash:  lastBlockHash,
		CommitState:     c.State,
		UnsealedBlocks:  make([]*BlockBuilder, 0),
		SealedBlocks:    make([]*SealedBlock, 0),
		RecentBlocks:    make([]*CommitBlock, 0),
		Checksum:        c,
	}
}

func (c *Chain) CheckpointValidator(header BlockHeader) (*BlockBuilder, error) {
	if header.Epoch <= c.LastCommitEpoch {
		return nil, errors.New("cannot replace commited block outside recovery mode")
	}
	if header.CheckPoint < c.Checksum.Epoch {
		return nil, errors.New("cannot have checkpoint before checksum")
	}
	builder := BlockBuilder{
		Header:  header,
		Actions: NewActionArray(),
	}
	mutations := make([]*state.Mutations, 0)
	if header.CheckPoint < c.LastCommitEpoch {
		for _, commit := range c.RecentBlocks {
			if commit.Header.Epoch >= c.Checksum.Epoch && commit.Header.Epoch <= header.CheckPoint {
				mutations = append(mutations, commit.mutations)
			}
			if commit.Header.Epoch > header.CheckPoint {
				break
			}
		}
		aggrMutations := state.NewMutations(header.Epoch).Append(mutations)
		builder.Validator = c.Checksum.State.Validator(aggrMutations, header.Epoch)
		return &builder, nil
	}
	for _, sealed := range c.SealedBlocks {
		if sealed.Header.Epoch > c.LastCommitEpoch && sealed.Header.Epoch <= header.CheckPoint {
			mutations = append(mutations, sealed.Mutations)
		}
		if sealed.Header.Epoch > header.CheckPoint {
			break
		}
	}
	aggrMutations := state.NewMutations(header.Epoch).Append(mutations)
	builder.Validator = c.CommitState.Validator(aggrMutations, header.Epoch)
	return &builder, nil
}

func (c *Chain) CommitBlock(blockEpoch uint64, publisher crypto.PrivateKey) bool {
	if blockEpoch != c.LastCommitEpoch+1 {
		return false // commit must be sequential
	}
	var block *SealedBlock
	for n, sealed := range c.SealedBlocks {
		if sealed.Header.Epoch == blockEpoch {
			block = sealed
			c.SealedBlocks = append(c.SealedBlocks[0:n], c.SealedBlocks[n+1:]...)
			break
		}
	}
	if block == nil {
		return false
	}
	epoch := block.Header.Epoch
	var validator *state.MutatingState
	if epoch != c.LastCommitEpoch {
		validator = c.CommitState.Validator(state.NewMutations(epoch), epoch)
	}
	commit := block.Revalidate(validator, publisher)
	if commit == nil {
		return false
	}
	c.RecentBlocks = append(c.RecentBlocks, commit)
	validator.Incorporate(publisher.PublicKey())
	c.LastCommitEpoch = block.Header.Epoch
	c.LastCommitHash = block.Seal.Hash
	//fmt.Printf("block %v commited: %v actions\n", block.Header.Epoch, block.Actions.Len())
	return true
}

func (c *Chain) Validate(action []byte) bool {
	epoch := actions.GetEpochFromByteArray(action)
	if epoch == 0 || epoch > c.LiveBlock.Header.Epoch || (epoch+MaxProtocolEpoch < c.LiveBlock.Header.Epoch) {
		return false
	}
	hash := crypto.Hasher(action)
	if !c.Incorporated.IsNew(hash, epoch, c.LiveBlock.Header.CheckPoint) {
		return false
	}
	return c.LiveBlock.Validate(action)
}

func (c *Chain) SealOwnBlock() {
	c.SealedBlocks = append(c.SealedBlocks, c.LiveBlock.Seal(c.Credentials))
	c.LiveBlock = nil
}

func (c *Chain) SealBlock(epoch uint64, hash crypto.Hash, signature crypto.Signature) error {
	var block *BlockBuilder
	if c.LiveBlock.Header.Epoch == epoch {
		if !hash.Equal(c.LiveBlock.Hash()) {
			return errors.New("hash does not match")
		}
		if !c.LiveBlock.Header.Proposer.Verify(hash[:], signature) {
			return errors.New("invalid signature")
		}
		c.LiveBlock = nil
		block = c.LiveBlock
	} else {
		for n, unsealed := range c.UnsealedBlocks {
			if unsealed.Header.Epoch == epoch {
				if !hash.Equal(unsealed.Hash()) {
					return errors.New("hash does not match")
				}
				if !unsealed.Header.Proposer.Verify(hash[:], signature) {
					return errors.New("invalid signature")
				}
				block = unsealed
				c.UnsealedBlocks = append(c.UnsealedBlocks[:n], c.UnsealedBlocks[n+1:]...)
				break
			}
		}
	}
	if block == nil {
		return errors.New("block not found")
	}
	sealed := &SealedBlock{
		Header:  block.Header,
		Actions: block.Actions,
		Seal: BlockSeal{
			Hash:          hash,
			SealSignature: signature,
			FeesCollected: block.Validator.FeesCollected,
		},
		Mutations: block.Validator.Mutations(),
	}
	c.SealedBlocks = append(c.SealedBlocks, sealed)
	return nil
}

func (c *Chain) Rollover(epoch uint64) error {
	if epoch < c.LastCommitEpoch {
		return errors.New("rollover to a previously commit only allowed in recovery mode")
	}

	sealedBefore := make([]*SealedBlock, 0)
	for _, sealed := range c.SealedBlocks {
		if sealed.Header.Epoch <= epoch {
			sealedBefore = append(sealedBefore, sealed)
		}
	}
	if len(sealedBefore) != len(c.SealedBlocks) {
		c.SealedBlocks = sealedBefore
	}

	unsealed := make([]*BlockBuilder, 0)
	for _, block := range c.UnsealedBlocks {
		if block.Header.Epoch <= epoch {
			unsealed = append(unsealed, block)
		}
	}
	if len(unsealed) != len(c.UnsealedBlocks) {
		c.UnsealedBlocks = unsealed
	}
	if c.LiveBlock.Header.Epoch > epoch {
		c.LiveBlock = nil
	}
	return nil
}

func (c *Chain) Recovery(epoch uint64) error {
	if epoch < c.Checksum.Epoch {
		return errors.New("cannot automatically recover to an epoch before current checksum")
	}
	if epoch >= c.LastCommitEpoch {
		return errors.New("dont need recovery to an epoch after last commit")
	}
	commit := make([]*CommitBlock, 0)
	mutations := make([]*state.Mutations, 0)
	for _, block := range c.RecentBlocks {
		if block.Header.Epoch <= epoch {
			commit = append(commit, block)
			mutations = append(mutations, block.mutations)
		}
		if block.Header.Epoch == epoch {
			c.LastCommitHash = block.Seal.Hash
		}
	}
	groupedMutations := state.NewMutations(epoch).Append(mutations)
	c.Checksum.State.IncorporateMutations(groupedMutations)
	c.RecentBlocks = commit
	c.LastCommitEpoch = epoch
	return nil
}

func (c *Chain) Shutdown() {
	c.CommitState.Shutdown()
	c.Checksum.State.Shutdown()
}
