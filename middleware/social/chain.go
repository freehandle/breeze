package social

import (
	"fmt"
	"log"
	"sync"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

const (
	StatusLive byte = iota
	StatusDone
	StatusSealed
	StatusCommit
)

type SocialBlock[M Merger[M]] struct {
	Epoch       uint64
	Checkpoint  uint64
	Origin      crypto.Hash
	Actions     *chain.ActionArray
	Invalidated []crypto.Hash
	Status      byte
	mutations   M
}

func (a *SocialBlock[M]) Clone() *SocialBlock[M] {
	return &SocialBlock[M]{
		Epoch:       a.Epoch,
		Checkpoint:  a.Checkpoint,
		Origin:      a.Origin,
		Actions:     a.Actions.Clone(),
		Invalidated: a.Invalidated,
		Status:      a.Status,
	}
}

func NewSocialBlockChain[M Merger[M], B Blocker[M]](state Stateful[M, B], epoch uint64) *SocialBlockChain[M, B] {
	return &SocialBlockChain[M, B]{
		mu:           sync.Mutex{},
		epoch:        epoch,
		validator:    state.Validator(),
		commit:       state,
		commitEpoch:  epoch,
		recentBlocks: make([]*SocialBlock[M], 0),
	}
}

type SocialBlockChain[M Merger[M], B Blocker[M]] struct {
	mu            sync.Mutex
	epoch         uint64              // live epoch
	live          *SocialBlock[M]     // live block
	validator     B                   //live validator
	commit        Stateful[M, B]      // commit state
	commitEpoch   uint64              // commit epoch
	recentBlocks  []*SocialBlock[M]   // at least since checksum point
	Transform     func([]byte) []byte // in case actions need to be transformed
	checksumEpoch uint64              // epoch of the last checksum for recovery
}

func (s *SocialBlockChain[M, B]) Lock() {
	s.mu.Lock()
}

func (s *SocialBlockChain[M, B]) Unlock() {
	s.mu.Unlock()
}

func (s *SocialBlockChain[M, B]) Validate(action []byte) bool {
	if s.Transform != nil {
		action = s.Transform(action)
	}
	if len(action) > 0 && s.live != nil {
		if s.validator.Validate(action) {
			s.live.Actions.Append(action)
			return true
		}
	}
	return false
}

func (s *SocialBlockChain[M, B]) NextBlock(epoch uint64) error {
	if epoch != s.epoch+1 {
		return fmt.Errorf("non-sequential block is not allowed: %d not next vs proposed next of %d", s.epoch, epoch)
	}
	if s.live != nil {
		s.live.Status = StatusDone
		s.recentBlocks = append(s.recentBlocks, s.live)
		s.live.mutations = s.validator.Mutations()
	}
	s.epoch += 1
	s.live = &SocialBlock[M]{
		Epoch:      s.epoch,
		Checkpoint: s.commitEpoch,
		Actions:    chain.NewActionArray(),
		Status:     StatusLive,
	}
	s.validator = s.commit.Validator()
	return nil
}

func (s *SocialBlockChain[M, B]) SealBlock(epoch uint64, hash crypto.Hash) (crypto.Hash, error) {
	var sealed *SocialBlock[M]
	if s.live != nil && s.live.Epoch == epoch {
		s.recentBlocks = append(s.recentBlocks, s.live)
		s.live = nil
	} else {
		sealed = s.findBlock(epoch)
	}
	if sealed == nil {
		return crypto.ZeroHash, fmt.Errorf("block %d not found", epoch)
	}
	if sealed.Status >= StatusSealed {
		return crypto.ZeroHash, fmt.Errorf("block %d already sealed", epoch)
	}
	sealed.Status = StatusSealed
	sealed.Origin = hash
	return sealed.Actions.Hash(), nil
}

func (s *SocialBlockChain[M, B]) findBlock(epoch uint64) *SocialBlock[M] {
	for _, block := range s.recentBlocks {
		if block.Epoch == epoch {
			return block
		}
	}
	return nil
}

func (s *SocialBlockChain[M, B]) revalidate(actions *chain.ActionArray, invalidated []crypto.Hash) ([]crypto.Hash, M) {
	notvalid := make(map[crypto.Hash]struct{})
	for _, hash := range invalidated {
		notvalid[hash] = struct{}{}
	}
	validator := s.commit.Validator()
	socialInvalidated := make([]crypto.Hash, 0)
	for n := 0; n < actions.Len(); n++ {
		action := actions.Get(n)
		hash := crypto.Hasher(action)
		if _, ok := notvalid[hash]; ok || !validator.Validate(action) {
			socialInvalidated = append(socialInvalidated, hash)
		}
	}
	return socialInvalidated, validator.Mutations()
}

func (s *SocialBlockChain[M, B]) Commit(epoch uint64, invalidated []crypto.Hash) ([]crypto.Hash, error) {
	for _, block := range s.recentBlocks {
		if block.Epoch == epoch {
			if block.Status == StatusCommit {
				return nil, fmt.Errorf("block %d already committed", epoch)
			} else if block.Status != StatusSealed {
				return nil, fmt.Errorf("block %d not sealed", epoch)
			}
			block.Status = StatusCommit
			block.Invalidated, block.mutations = s.revalidate(block.Actions, invalidated)
			s.commit.Incorporate(block.mutations)
			s.commitEpoch = block.Epoch
			return block.Invalidated, nil
		} else if block.Status != StatusCommit {
			return nil, fmt.Errorf("non-sequential commit is not allowed: %d not commit vs proposed commit of %d", block.Epoch, epoch)
		}
	}
	return nil, fmt.Errorf("block %d not found", epoch)
}

func (s *SocialBlockChain[M, B]) Rollback(epoch uint64) error {
	if epoch < s.commitEpoch {
		return fmt.Errorf("Rollback request to a commit epoch: rollback to %v vs commit %v", epoch, s.commitEpoch)
	}
	for n, block := range s.recentBlocks {
		if block.Epoch == epoch {
			s.recentBlocks = s.recentBlocks[:n]
			s.live = nil
			s.epoch = epoch
			return nil
		}
	}
	return fmt.Errorf("Rollback: could not find block %v", epoch)
}

func (s *SocialBlockChain[M, B]) Recovery(epoch uint64) {
	if epoch < s.checksumEpoch {
		log.Printf("Recovery request to an epoch before checksum: rollback to %v vs checksum epoch %v", epoch, s.checksumEpoch)
		return
	}
	if epoch >= s.commitEpoch {
		log.Printf("Recovery request to an epoch after commit epoch: rollback to %v vs commit epoch %v", epoch, s.commitEpoch)
		return
	}
	var mutations M
	for n, block := range s.recentBlocks {
		if block.Epoch == s.checksumEpoch+1 {
			mutations = block.mutations

		} else if block.Epoch > s.checksumEpoch+1 && block.Epoch <= epoch {
			mutations.Merge(block.mutations)
		}
		if block.Epoch == epoch {
			s.recentBlocks = s.recentBlocks[:n]
			break
		}
	}
	s.commit.Recover()
	s.commit.Incorporate(mutations)
	s.commitEpoch = epoch
	s.live = nil
}

func (c *SocialBlockChain[M, B]) Sync(conn *socket.CachedConnection, epoch uint64) {
	syncBlocks := make([]*SocialBlock[M], 0)
	for _, block := range c.recentBlocks {
		if block.Epoch > epoch && block.Epoch <= c.epoch {
			syncBlocks = append(syncBlocks, block)
		}
	}
	if c.live != nil {
		syncBlocks = append(syncBlocks, c.live.Clone())
	}
	go func() {
		for _, block := range syncBlocks {
			bytes := []byte{messages.MsgNewBlock}
			util.PutUint64(block.Epoch, &bytes)
			conn.SendDirect(bytes)
			bytes = block.Actions.Serialize()
			conn.SendDirect(append([]byte{messages.MsgActionArray}, bytes...))
			if block.Status >= StatusSealed {
				bytes := []byte{messages.MsgSeal}
				util.PutUint64(block.Epoch, &bytes)
				util.PutHash(block.Actions.Hash(), &bytes)
				conn.SendDirect(bytes)
			}
			if block.Status >= StatusCommit {
				bytes := []byte{messages.MsgCommit}
				util.PutUint64(block.Epoch, &bytes)
				util.PutHashArray(block.Invalidated, &bytes)
				conn.SendDirect(bytes)
			}
		}
		conn.Ready()
	}()
}
