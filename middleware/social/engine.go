package social

import (
	"context"

	"github.com/freehandle/breeze/consensus/chain"
)

type Engine[T Merger[T], B Blocker[T]] struct {
	blockchain *SocialBlockChain[T, B]
	block      chan *SocialBlock
	commit     chan *SocialBlockCommit
	forward    chan []byte
}

func NewEngine[M Merger[M], B Blocker[M]](ctx context.Context, blockchain *SocialBlockChain[M, B]) *Engine[M, B] {
	engine := &Engine[M, B]{
		blockchain: blockchain,
		block:      make(chan *SocialBlock),
		commit:     make(chan *SocialBlockCommit),
		forward:    make(chan []byte),
	}
	go func() {
		done := ctx.Done()
		for {
			select {
			case <-done:
				return
			case block := <-engine.block:
				if block != nil {
					incorporated := engine.blockchain.AddAncestorBlock(block)
					if incorporated != nil {
						engine.forward <- incorporated.Block.Serialize()
					}
					pending := engine.blockchain.CommitAll()
					for _, block := range pending {
						if commit := block.Block.Commit(); commit != nil {
							engine.forward <- commit.Serialize()
						}
					}
				}
			case commit := <-engine.commit:
				if commit != nil {
					block := engine.blockchain.AddAncestorCommit(commit)
					if block != nil {
						if commit := block.Block.Commit(); commit != nil {
							engine.forward <- commit.Serialize()
						}
					}
					pending := engine.blockchain.CommitAll()
					for _, block := range pending {
						if commit := block.Block.Commit(); commit != nil {
							engine.forward <- commit.Serialize()
						}
					}
				}
			}
		}
	}()

	return engine
}

func BreezeSealedBlockToSocialBlock(block *chain.SealedBlock) *SocialBlock {
	return &SocialBlock{
		ProtocolCode:  0,
		Epoch:         block.Header.Epoch,
		Checkpoint:    block.Header.CheckPoint,
		BreezeBlock:   block.Seal.Hash,
		Pedigree:      nil,
		Actions:       block.Actions,
		SealHash:      block.Seal.Hash,
		SealSignature: block.Seal.SealSignature,
		Status:        Sealed,
	}
}

func BreezeComiittedBlockToSocialBlock(block *chain.CommitBlock) *SocialBlock {
	return &SocialBlock{
		ProtocolCode: 0,
		Epoch:        block.Header.Epoch,
		Checkpoint:   block.Header.CheckPoint,
		BreezeBlock:  block.Seal.Hash,
		Pedigree:     nil,
		Actions:      block.Actions,
		Invalidated:  block.Commit.Invalidated,
		SealHash:     block.Seal.Hash,
		CommitHash:   block.Seal.Hash,
		Status:       Committed,
	}
}
