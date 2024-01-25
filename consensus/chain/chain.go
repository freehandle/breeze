/*
	chain provides blockchain interface

Rules for the chain mechanism:

 1. blocks are proposed for a certain epoch and against a certain checkpoint
    prior to that epoch.
 2. the block associated to a checkpoint must be sealed, otherwise it is not a
    valid checkpoint. sealed blocks cannot append new actions. They are not
    considerer final because certain actions can be removed by the commit phase.
 3. actions for the block are temporarily validated against the state derived
    at the checkpoint epoch.
 4. blocks are sealed, a hash is calculated, and the hash is signed by the
    publisher of the block. the commit phase is done by every node
 5. blocks are commited with all transactions validated with the checkpoint of
    the epoch immediately before the block epoch. Actions that were approved as
    validated by the original checkpoint are marked as invalidated by the commit
    instruction.
 6. state the system can be synced to another node.
*/
package chain

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/state"
)

// block chain should keep as many KeepLastNBlocks blocks in memory for fast
// sync jobs and for recovery purposes.
const KeepLastNBlocks = 100

// Force and Empty commit after so many blocks stagnated on a given checkpoint.
// the Block on the following checkpoint is forced to be empty and unsigned.
const ForceEmptyCommitAfter = 20

// Checksum is a snapshot of the state of the system at periodic epochs. It
// contains the epoch, the state of the chain at that epoch, the hash of the
// last sealed block at that epoch and a checksum hash for the state of the
// system. Only validators which agree and provide evidence of the same state
// checksum are allowed to participate in the next checksum window.
type Checksum struct {
	Epoch         uint64
	State         *state.State
	LastBlockHash crypto.Hash
	Hash          crypto.Hash
}

// The last perceived epoch-to-clock synchronization in possession of the node.
// The first synchronization is at genesis time. If there is no downtime,
// this synchronization remains in place. If there is downtime, the nodes
type ClockSyncronization struct {
	Epoch     uint64
	TimeStamp time.Time
}

func (c ClockSyncronization) Timer(d time.Duration) time.Duration {
	now := time.Now()
	cycles := now.Sub(c.TimeStamp) / d
	nextCycle := c.TimeStamp.Add((cycles + 1) * d)
	return time.Until(nextCycle)
}

// Blockchain is the main data structure for the breeze network. It contains the
// state of the system at the last commit and the sealed uncommited blocks.
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
	NextChecksum    *Checksum
	Clock           ClockSyncronization
	Punishment      map[crypto.Token]uint64
	BlockInterval   time.Duration
	ChecksumWindow  int
}

// IsChecksumEpoch returns true if the provided epoch is a checksum epoch and
// the checksum is not yet available. It returns false otherwise.
func (b *Blockchain) IsChecksumCommit() bool {
	if (int(b.LastCommitEpoch) % b.ChecksumWindow) == (b.ChecksumWindow / 2) {
		epoch := int(b.LastCommitEpoch)/b.ChecksumWindow + b.ChecksumWindow/2
		if int(b.Checksum.Epoch) != epoch {
			return true
		}
	}
	return false
}

// BlockchainFromGenesisState creates a new blockchain from a genesis state. It
// creates the genesis state and depoits the initial tokens on the credentials
// wallet. Credentials is also accredited as a validator for the first checksum
// window. hash is the network hash. interval is the block interval. checksum
// window is the number of epochs between checksums.
func BlockchainFromGenesisState(credentials crypto.PrivateKey, walletPath string, hash crypto.Hash, interval time.Duration, cehcksumWindow int) *Blockchain {
	genesis := state.NewGenesisStateWithToken(credentials.PublicKey(), walletPath)
	if genesis == nil {
		return nil
	}
	cloned := genesis.Clone()
	blockchain := &Blockchain{
		mu:              sync.Mutex{},
		NetworkHash:     hash,
		Credentials:     credentials,
		LastCommitEpoch: 0,
		LastCommitHash:  crypto.HashToken(credentials.PublicKey()),
		CommitState:     genesis,
		SealedBlocks:    make([]*SealedBlock, 0),
		RecentBlocks:    make([]*CommitBlock, 0),
		Checksum: &Checksum{
			Epoch:         0,
			State:         cloned,
			LastBlockHash: crypto.HashToken(credentials.PublicKey()),
			Hash:          cloned.ChecksumHash(),
		},
		Clock: ClockSyncronization{
			Epoch:     0,
			TimeStamp: time.Now(),
		},
		BlockInterval:  interval,
		ChecksumWindow: cehcksumWindow,
	}
	slog.Info("gensis state created", "token", credentials.PublicKey(), "genesis hash", crypto.EncodeHash(blockchain.Checksum.Hash))
	return blockchain
}

// TimestampBlock returns the time at which a block with the provided epoch
// will start. It is calculated from the last clock synchronization and the
// block interval.
func (b *Blockchain) TimestampBlock(epoch uint64) time.Time {
	delta := time.Duration(epoch-b.Clock.Epoch) * b.BlockInterval
	return b.Clock.TimeStamp.Add(delta)
}

// Timer returns a timer that will fire at the time at which a block with the
// provided epoch will start. It is calculated from the last clock
func (b *Blockchain) Timer(epoch uint64) *time.Timer {
	delta := time.Until(b.TimestampBlock(epoch))
	return time.NewTimer(delta)
}

// BlockchainFromChecksumState recreates a blockchain from a given checksum
// state. Checksum state typically comes from a peer node sync job.
func BlockchainFromChecksumState(c *Checksum, clock ClockSyncronization, credentials crypto.PrivateKey, networkHash crypto.Hash, interval time.Duration, checksumWindow int) *Blockchain {
	blockchain := &Blockchain{
		mu:              sync.Mutex{},
		NetworkHash:     networkHash,
		Credentials:     credentials,
		LastCommitEpoch: c.Epoch,
		LastCommitHash:  c.LastBlockHash,
		CommitState:     c.State,
		SealedBlocks:    make([]*SealedBlock, 0),
		RecentBlocks:    make([]*CommitBlock, 0),
		Checksum:        c,
		Clock:           clock,
		BlockInterval:   interval,
		ChecksumWindow:  checksumWindow,
	}
	slog.Info("blockchain created", "check epoch", blockchain.Checksum.Epoch, "checksum hash", crypto.EncodeHash(blockchain.Checksum.Hash))
	return blockchain
}

func (c *Blockchain) RecentAfter(epoch uint64) []*CommitBlock {
	recent := make([]*CommitBlock, 0)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, block := range c.RecentBlocks {
		if block.Header.Epoch > epoch {
			recent = append(recent, block)
		}
	}
	return recent
}

// NextBlock returns a block header for the next block to be proposed. It
// retrieves the checkpoint from the last commit state.
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

// CheckpointValidator returns a block builder for a block that will validate
// actions proposed. It returns nil if the proposed epoch in the header is for
// a period prior to the last commit epoch.
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

// AddSealedBlock adds a sealed block to the blockchain. If the added block can
// be commit it is automatically committed and subsequent sealed blocks are
// committed as well. The committ machanism will trigger a checksum job if any
// epoch matches the checksum window. If not committed the block is kept as a
// sealed block. Sealed block are expected to have consensus evidence.
// AddSealedBlock does not check for consensus evidence, or valid signature. It
// is the reponsability of the caller to ensure that the sealed block has all
// necessary and valid information within it.
func (c *Blockchain) AddSealedBlock(sealed *SealedBlock) {
	if sealed == nil {
		return
	}
	if sealed.Header.Epoch <= c.LastCommitEpoch {
		slog.Warn("Blockchain: cannot add sealed block before last commit")
		return
	}
	hasFound := false
	for n, existing := range c.SealedBlocks {
		if existing.Header.Epoch == sealed.Header.Epoch {
			slog.Warn("Blockchain: cannot add sealed block for existing epoch")
			return
		} else if existing.Header.Epoch > sealed.Header.Epoch {
			c.SealedBlocks = append(c.SealedBlocks[0:n], append([]*SealedBlock{sealed}, c.SealedBlocks[n:]...)...)
			hasFound = true
			break
		}
	}
	if !hasFound {
		c.SealedBlocks = append(c.SealedBlocks, sealed)
	}
	slog.Info("Blockchain: added sealed block", "epoch", sealed.Header.Epoch, "hash", crypto.EncodeHash(sealed.Seal.Hash), "publisher", sealed.Header.Proposer)
	if !c.Cloning {
		c.CommitChain()
	}
}

// CommitChain commits all sequential sealed blocks in the blockchain following a
// last commit epoch. If stops the commit chain if the commit epoch is a
// checksum commit epoch. It is the responsibility of the caller to ensure that
// the checksum job request is called.
func (c *Blockchain) CommitChain() {
	for {
		if len(c.SealedBlocks) == 0 {
			return
		}
		if c.CheckForceEmptyCommit() {
			continue
		}
		if c.SealedBlocks[0].Header.Epoch == c.LastCommitEpoch+1 {
			if !c.CommitBlock(c.LastCommitEpoch + 1) {
				return
			}
		} else {
			break
		}
	}
}

// CheckForceEmptyCommit forces an empty unsigned commit to the blockchain. This
// is allowed when MaxLag sucessive blocks have been sealed but not commited, due
// to a missing LastCommitEpoch + 1 block.
func (c *Blockchain) CheckForceEmptyCommit() bool {
	if len(c.SealedBlocks) < ForceEmptyCommitAfter || c.SealedBlocks[0].Header.Epoch == c.LastCommitEpoch+1 {
		return false
	}
	count := 0
	for n := 0; n < len(c.SealedBlocks); n++ {
		if (c.SealedBlocks[n].Header.CheckPoint == c.LastCommitEpoch) && (c.SealedBlocks[n].Header.CheckpointHash.Equal(c.LastCommitHash)) {
			count += 1
		}
		if count >= ForceEmptyCommitAfter {
			break
		}
	}
	if count < ForceEmptyCommitAfter {
		return false
	}
	header := BlockHeader{
		NetworkHash:    c.NetworkHash,
		Epoch:          c.LastCommitEpoch + 1,
		CheckPoint:     c.LastCommitEpoch,
		CheckpointHash: c.LastCommitHash,
		Proposer:       crypto.ZeroToken,
		ProposedAt:     time.Unix(0, 0),
	}
	headerHash := crypto.Hasher(header.Serialize())
	commit := CommitBlock{
		Header:  header,
		Actions: NewActionArray(),
		Seal: BlockSeal{
			Hash:          crypto.Hasher(append(headerHash[:], crypto.ZeroValueHash[:]...)),
			FeesCollected: 0,
			SealSignature: crypto.ZeroSignature,
		},
		Commit: &BlockCommit{
			Invalidated:   make([]crypto.Hash, 0),
			FeesCollected: 0,
			PublishedBy:   c.Credentials.PublicKey(),
		},
	}
	bytes := commit.serializeForPublish()
	commit.Commit.PublishSign = c.Credentials.Sign(bytes)
	c.RecentBlocks = append(c.RecentBlocks, &commit)
	c.LastCommitEpoch += 1
	c.LastCommitHash = commit.Seal.Hash
	return true
}

// CommitBlock commits a sealed block to the blockchain. It returns true if the
// block was successfully committed. It returns false if the block was not
// committed. It is not committed if the block epoch is not sequential to the
// last commit epoch, if the block epoch is already committed, or if the block
// epoch is not found in the sealed blocks.
func (c *Blockchain) CommitBlock(blockEpoch uint64) bool {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("chain CommitBlock panic", "err", r)
		}
	}()
	if c.Cloning {
		return false
	}
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
	if c.IsChecksumCommit() {
		c.MarkCheckpoint()
	}
	slog.Info("Blockchain: committed block", "epoch", block.Header.Epoch, "hash", crypto.EncodeHash(block.Seal.Hash), "actions", commit.Actions.Len(), "invalidated", len(commit.Commit.Invalidated))
	if blockEpoch%uint64(c.ChecksumWindow) == 0 {
		c.Checksum = c.NextChecksum
		c.NextChecksum = nil
		slog.Info("Breeze: checksum window completed", "epoch", block.Header.Epoch, "last block hash", crypto.EncodeHash(block.Seal.Hash))
	}
	return true
}

// Rollover rolls over the blockchain to a previous epoch after the last commit
// epoch. It erases any sealed block. It returns an error if the epoch is before
// the last commit epoch.
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

// Recovery rolls over the blockchain to a previous epoch before the last commit
// but after the last checksum epoch. It erases any sealed block or commit block
// and recalculates the state of the chain at the new epoch.
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
