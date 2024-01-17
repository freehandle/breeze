package social

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
)

const (
	StatusLive byte = iota
	StatusDone
	StatusSealed
	StatusCommit
)

type SocialBlockChainConfig struct {
	Credentials  crypto.PrivateKey
	ProtocolCode uint32
	Transform    func([]byte) []byte // action transformation if any
	KeepNBlocks  int                 // keep the most N recent blocks
	StateLag     uint64              // evolve the state with a lag
}

type Checksum[T Merger[T], B Blocker[T]] struct {
	Epoch         uint64
	State         Stateful[T, B]
	LastBlockHash crypto.Hash
	Hash          crypto.Hash
}

// SocialBlockChain keeps the evolution of the state and mutations.
type SocialBlockChain[M Merger[M], B Blocker[M]] struct {
	mu             sync.Mutex
	stateLag       uint64
	Clock          chain.ClockSyncronization
	ProtocolCode   uint32
	ProtocolFilter func([]byte) bool
	Credentials    crypto.PrivateKey
	stateEpoch     uint64
	lastCommit     uint64
	state          Stateful[M, B]              // commit state
	recentBlocks   []*SocialBlockBuilder[M, B] // at least since checksum point
	ancestorCommit []*SocialBlockCommit        // ancestor commits not yet incorporated
	ancestorBlock  []*SocialBlock              // ancestor blocks not yet incorporated
	Transform      func([]byte) []byte         // in case actions need to be transformed
	Checksum       *Checksum[M, B]
	nextChecksum   *Checksum[M, B]
}

// Create new blockchain from checksum epoch and state according to config
// parametrs.
func NewSocialBlockChain[M Merger[M], B Blocker[M]](config Configuration, checksum *Checksum[M, B]) *SocialBlockChain[M, B] {
	return &SocialBlockChain[M, B]{
		mu:           sync.Mutex{},
		stateLag:     config.MaxCheckpointLag,
		Credentials:  config.Credentials,
		stateEpoch:   checksum.Epoch,
		lastCommit:   checksum.Epoch,
		state:        checksum.State,
		recentBlocks: make([]*SocialBlockBuilder[M, B], 0),
		Checksum:     checksum,
	}
}

// MutationsBetween returns consecutive block mutations between start and finish
// epochs with status equal or greater than status. If there are not as many
// blocks as requested or gaps in between, it returns nil and false
func (s *SocialBlockChain[M, B]) MutationsBetween(start, finish uint64, status byte) ([]M, bool) {
	started := false
	mutations := make([]M, 0)
	lastAppend := uint64(0)
	for _, recent := range s.recentBlocks {
		if recent.Block.Epoch == start {
			if recent.Block.Status < status {
				return nil, false
			}
			started = true
			mutations = append(mutations, recent.Validator.Mutations())
			lastAppend = recent.Block.Epoch
			if finish == start {
				return mutations, true
			}
		} else if started {
			// gap in the recent block chain
			if recent.Block.Epoch != lastAppend+1 || recent.Block.Status < status {
				return nil, false
			}
			mutations = append(mutations, recent.Validator.Mutations())
			lastAppend = recent.Block.Epoch
			if lastAppend == finish {
				return mutations, true
			}
		}
	}
	// insufficient blocks to get until finish
	return nil, false
}

// returns state validator for the checkpoint if there are enough committed
// blocks to get to the checkpoint. Otherwise, it returns false.
func (s *SocialBlockChain[M, B]) GetValidator(checkpoint uint64) (B, bool) {
	var validator B
	if checkpoint == s.stateEpoch {
		return s.state.Validator(), true
	}
	mutations, ok := s.MutationsBetween(s.stateEpoch+1, checkpoint, Committed)
	if !ok {
		return validator, false
	}
	return s.state.Validator(mutations...), true
}

func (s *SocialBlockChain[M, B]) IncorporateAll() []*SocialBlockBuilder[M, B] {
	incorporated := make([]*SocialBlockBuilder[M, B], 0)
	remaining := make([]*SocialBlock, 0)
	for _, block := range s.ancestorBlock {
		if block.Checkpoint <= s.lastCommit {
			added := s.AddAncestorBlock(block)
			if added != nil {
				incorporated = append(incorporated, added)
			} else {
				remaining = append(remaining, block)
			}
		}
	}
	if len(incorporated) > 0 {
		s.ancestorBlock = remaining
	}
	return incorporated
}

func (s *SocialBlockChain[M, B]) AddAncestorBlock(block *SocialBlock) *SocialBlockBuilder[M, B] {
	sealCheckpoint, ok := s.GetValidator(block.Checkpoint)
	if !ok {
		s.ancestorBlock = append(s.ancestorBlock, block)
		return nil
	}
	builder := &SocialBlockBuilder[M, B]{
		Block: &SocialBlock{
			ProtocolCode: s.ProtocolCode,
			Epoch:        block.Epoch,
			Checkpoint:   block.Checkpoint,
			BreezeBlock:  block.BreezeBlock,
			Pedigree:     block.Pedigree, // updated only after commit
			Actions:      chain.NewActionArray(),
			Invalidated:  make([]crypto.Hash, 0),
			Publisher:    s.Credentials.PublicKey(),
			Status:       Sealed,
		},
		Validator: sealCheckpoint,
	}
	for n := 0; n < block.Actions.Len(); n++ {
		action := block.Actions.Get(n)
		if s.ProtocolFilter(action) {
			builder.Block.Actions.Append(action)
		}
	}
	builder.Block.Seal(s.Credentials)
	if block.Status >= Committed {
		commitCheckpoint, ok := s.GetValidator(block.Checkpoint)
		if !ok {
			return builder
		}
		builder.Validator = commitCheckpoint
		builder.Revalidate(block.Invalidated, s.Credentials)
		ancestor := Ancestor{
			ProtocolCode:    s.ProtocolCode,
			Publisher:       block.Publisher,
			CommitHash:      block.CommitHash,
			CommitSignature: block.CommitSignature,
		}
		builder.Block.Pedigree = append(builder.Block.Pedigree, ancestor)
		builder.Block.Status = Committed
		if s.lastCommit < block.Epoch {
			s.lastCommit = block.Epoch
		}
	}
	return builder
}

func (s *SocialBlockChain[M, B]) appendCommit(commit *SocialBlockCommit) {
	if len(s.ancestorCommit) == 0 {
		s.ancestorCommit = append(s.ancestorCommit, commit)
		return
	}
	for n, ancestor := range s.ancestorCommit {
		if ancestor.Epoch > commit.Epoch {
			s.ancestorCommit = append(s.ancestorCommit[:n], append([]*SocialBlockCommit{commit}, s.ancestorCommit[n:]...)...)
			return
		}
	}
}

// Commit tries to commit
func (s *SocialBlockChain[M, B]) AddAncestorCommit(commit *SocialBlockCommit) *SocialBlockBuilder[M, B] {
	var block *SocialBlockBuilder[M, B]
	for _, recent := range s.recentBlocks {
		if recent.Block.Epoch == commit.Epoch && recent.Block.SealHash.Equal(commit.SealHash) {
			if recent.Block.Status >= Committed {
				return nil
			} else if recent.Block.Status == Unsealed {
				recent.Block.Seal(s.Credentials)
			}
			block = recent
			break
		}
	}
	if block == nil {
		s.appendCommit(commit)
		return nil
	}
	checkpoint, ok := s.GetValidator(commit.Epoch - 1)
	if !ok {
		s.appendCommit(commit)
		return nil
	}
	block.Validator = checkpoint
	block.Revalidate(commit.Invalidated, s.Credentials)
	block.Block.Pedigree = append(block.Block.Pedigree, Ancestor{
		ProtocolCode:    commit.ProtocolCode,
		Publisher:       commit.Publisher,
		CommitHash:      commit.CommitHash,
		CommitSignature: commit.CommitSignature,
	})
	if s.lastCommit < block.Block.Epoch {
		s.lastCommit = block.Block.Epoch
	}
	return block
}

func (s *SocialBlockChain[M, B]) CommitAll() []*SocialBlockBuilder[M, B] {
	committed := make([]*SocialBlockBuilder[M, B], 0)
	remaining := make([]*SocialBlockCommit, 0)
	for n, commit := range s.ancestorCommit {
		if commit.Epoch == s.lastCommit+1 {
			block := s.AddAncestorCommit(commit)
			if block == nil {
				slog.Error("SocialBlockChain: CommitAll cannot commit next to lastCheckpoint", "epoch", s.lastCommit)
				remaining = append(remaining, commit)
			} else {
				committed = append(committed, block)
			}
		} else {
			remaining = append(remaining, s.ancestorCommit[n:]...)
		}
	}
	if len(committed) > 0 {
		s.ancestorCommit = remaining
	}
	return committed
}

func (s *SocialBlockChain[M, B]) UpdateState(current uint64) {
	if current > s.stateEpoch+s.stateLag {
		if mutations, ok := s.MutationsBetween(s.stateEpoch+1, current-s.stateLag, Committed); ok {
			if len(mutations) == 1 {
				s.state.Incorporate(mutations[0])
			} else if len(mutations) > 1 {
				s.state.Incorporate(mutations[0].Merge(mutations[1:]...))
			}
			s.stateEpoch = current - s.stateLag
		}
	}
}

func (s *SocialBlockChain[M, B]) Rollback(epoch uint64) error {
	if epoch < s.lastCommit {
		return fmt.Errorf("Rollback request to a commit epoch: rollback to %v vs commit %v", epoch, s.lastCommit)
	}
	for n, block := range s.recentBlocks {
		if block.Block.Epoch >= epoch {
			s.recentBlocks = s.recentBlocks[:n]
			return nil
		}
	}

	return fmt.Errorf("Rollback: could not find block %v", epoch)
}

func (s *SocialBlockChain[M, B]) Recovery(epoch uint64) error {
	if epoch < s.Checksum.Epoch {
		return fmt.Errorf("recovery request to an epoch before checksum: rollback to %v vs checksum epoch %v", epoch, s.Checksum.Epoch)
	}
	if epoch >= s.lastCommit {
		return fmt.Errorf("recovery request to an epoch after commit epoch: rollback to %v vs commit epoch %v", epoch, s.lastCommit)
	}
	mutations, ok := s.MutationsBetween(s.Checksum.Epoch+1, epoch, Committed)
	if !ok {
		return fmt.Errorf("recovery: could not find mutations between %v and %v", s.Checksum.Epoch+1, epoch)
	}
	s.state = <-s.Checksum.State.Clone()
	if len(mutations) == 1 {
		s.state.Incorporate(mutations[0])
	} else {
		s.state.Incorporate(mutations[0].Merge(mutations[1:]...))
	}
	s.lastCommit = epoch
	s.stateEpoch = epoch
	for n, block := range s.recentBlocks {
		if block.Block.Epoch >= epoch {
			s.recentBlocks = s.recentBlocks[:n]
			return nil
		}
	}
	return nil
}

func (s *SocialBlockChain[M, B]) Checkpoint() chan error {
	job := make(chan error, 2)
	if s.nextChecksum != nil {
		job <- errors.New("SocialBlockChain: Checkpoint called twice without Commit")
		return job
	}
	var lastBlock *SocialBlockBuilder[M, B]
	for _, block := range s.recentBlocks {
		if block.Block.Epoch == s.stateEpoch {
			if block.Block.Status == Committed {
				lastBlock = block
			}
			break
		}
	}
	if lastBlock == nil {
		job <- errors.New("SocialBlockChain: Checkpoint called on uncommitted block")
		return job
	}

	go func(epoch uint64, lastBlockHash crypto.Hash) {
		clone := <-s.state.Clone()
		hash := clone.Checksum()
		checksum := &Checksum[M, B]{
			Epoch:         s.stateEpoch,
			State:         clone,
			LastBlockHash: lastBlockHash,
			Hash:          hash,
		}
		s.nextChecksum = checksum
		job <- nil
	}(s.stateEpoch, lastBlock.Block.CommitHash)
	return job
}

func (c *SocialBlockChain[M, B]) NextWindow() {
	if c.nextChecksum == nil {
		slog.Error("SocialBlockChain: NextWindow called without Checkpoint")
		return
	}
	c.Checksum.State.Shutdown()
	c.Checksum = c.nextChecksum
	c.nextChecksum = nil
}
