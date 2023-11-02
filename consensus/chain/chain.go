package chain

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/state"
)

type Blockchain struct {
	mu              sync.Mutex
	NetworkHash     crypto.Hash
	Credentials     crypto.PrivateKey
	LastCommitEpoch uint64
	LastCommitHash  crypto.Hash
	CommitState     *state.State
	SealedBlocks    []*SealedBlock
	RecentBlocks    []*CommitBlock
	Cloning         bool
	Checksum        *Checksum
	Clock           ClockSyncronization
}

func BLockchainFromGenesisState(credentials crypto.PrivateKey, walletPath string) *Blockchain {
	genesis := state.NewGenesisStateWithToken(credentials.PublicKey(), walletPath)
	if genesis == nil {
		return nil
	}
	return &Blockchain{
		mu:              sync.Mutex{},
		NetworkHash:     crypto.HashToken(credentials.PublicKey()),
		Credentials:     credentials,
		LastCommitEpoch: 0,
		LastCommitHash:  crypto.HashToken(credentials.PublicKey()),
		CommitState:     genesis,
		SealedBlocks:    make([]*SealedBlock, 0),
		RecentBlocks:    make([]*CommitBlock, 0),
		Checksum: &Checksum{
			Epoch: 0,
			State: genesis,
			Hash:  crypto.HashToken(credentials.PublicKey()),
		},
	}
}

func BlockchainFromChecksumState(c *Checksum, credentials crypto.PrivateKey, lastBlockHash crypto.Hash) *Blockchain {
	return &Blockchain{
		mu:              sync.Mutex{},
		Credentials:     credentials,
		LastCommitEpoch: c.Epoch,
		LastCommitHash:  lastBlockHash,
		CommitState:     c.State,
		SealedBlocks:    make([]*SealedBlock, 0),
		RecentBlocks:    make([]*CommitBlock, 0),
		Checksum:        c,
	}
}

func (c *Blockchain) NextBlock(epoch uint64) *BlockHeader {
	if epoch <= c.LastCommitEpoch {
		slog.Info("Blockchain: NextBlock: epoch already commited")
		return nil
	}
	for _, sealed := range c.SealedBlocks {
		if sealed.Header.Epoch == epoch {
			slog.Info("Blockchain: NextBlock: epoch already sealed")
			return nil
		}
	}
	return &BlockHeader{
		NetworkHash:    c.NetworkHash,
		Epoch:          epoch,
		CheckPoint:     c.LastCommitEpoch,
		CheckpointHash: c.LastCommitHash,
		Proposer:       c.Credentials.PublicKey(),
		ProposedAt:     time.Now(),
	}
}

func (c *Blockchain) CheckpointValidator(header BlockHeader) *BlockBuilder {
	if header.Epoch <= c.LastCommitEpoch {
		slog.Warn("CheckpointValidator: cannot replace commited block outside recovery mode")
		return nil
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
		return &builder
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
	return &builder
}

func (c *Blockchain) AddSealedBlock(sealed *SealedBlock) {
	if sealed == nil {
		return
	}
	if sealed.Header.Epoch <= c.LastCommitEpoch {
		slog.Warn("Blockchain: AddSealedBlock: cannot add sealed block before last commit")
		return
	}
	for _, existing := range c.SealedBlocks {
		if existing.Header.Epoch == sealed.Header.Epoch {
			slog.Warn("Blockchain: AddSealedBlock: cannot add sealed block for existing epoch")
			return
		}
	}
	c.SealedBlocks = append(c.SealedBlocks, sealed)
	if sealed.Header.Epoch == c.LastCommitEpoch+1 {
		c.CommitAll()
	}
}

func (c *Blockchain) CommitAll() {
	for {
		exists := false
		for _, sealed := range c.SealedBlocks {
			if sealed.Header.Epoch == c.LastCommitEpoch+1 {
				exists = true
				c.CommitBlock(sealed.Header.Epoch)
				break
			}
		}
		if !exists {
			return
		}
	}
}

func (c *Blockchain) CommitBlock(blockEpoch uint64) bool {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("chain CommitBlock panic", "err", r)
		}
	}()
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
	commit := block.Revalidate(validator, c.Credentials)
	if commit == nil {
		return false
	}
	c.RecentBlocks = append(c.RecentBlocks, commit)
	validator.Incorporate(c.Credentials.PublicKey())
	c.LastCommitEpoch = block.Header.Epoch
	c.LastCommitHash = block.Seal.Hash
	return true
}

func (c *Blockchain) Rollover(epoch uint64) error {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("chain Rollover panic", "err", r)
		}
	}()
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
	return nil
}

func (c *Blockchain) Recovery(epoch uint64) error {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("chain Recovery panic", "err", r)
		}
	}()
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

func (c *Blockchain) Shutdown() {
	if c.CommitState != nil {
		c.CommitState.Shutdown()
	}
	if c.Checksum != nil && c.Checksum.State != nil {
		c.Checksum.State.Shutdown()
	}
}
