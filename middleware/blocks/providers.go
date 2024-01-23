package blocks

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/blockdb"
	"github.com/freehandle/breeze/middleware/social"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

func stripOld(blocks []*social.SocialBlock, starting uint64) []*social.SocialBlock {
	stripped := make([]*social.SocialBlock, 0)
	for _, block := range blocks {
		if block.Epoch >= starting {
			stripped = append(stripped, block)
		}
	}
	return stripped
}

func SocialIndexerProvider(ctx context.Context, protocol uint32, sources *socket.TrustedAggregator, idx func([]byte) []crypto.Hash) *util.Chain[*blockdb.IndexedBlock] {

	blocks := make(chan *social.SocialBlock)
	commits := make(chan *social.SocialBlockCommit)

	ctxC, cancel := context.WithCancel(ctx)
	social.SocialProtocolBlockListener(ctxC, protocol, sources, blocks, commits)

	chain := util.NewChain[*blockdb.IndexedBlock](ctx, 0)

	uncommitted := make([]*social.SocialBlock, 0)

	go func() {
		defer cancel()
		for {
			done := ctx.Done()
			select {
			case <-done:
				return
			case block := <-blocks:
				if block.CommitSignature == crypto.ZeroSignature {
					uncommitted = append(uncommitted, block)
				} else {
					chain.Push(&blockdb.IndexedBlock{
						Epoch: block.Epoch,
						Data:  block.Serialize(),
						Items: socialCommitToIndex(block, idx),
					}, block.Epoch)
				}
			case commit := <-commits:
				for n, block := range uncommitted {
					if block.Epoch == commit.Epoch {
						if commit.SealHash == block.SealHash {
							uncommitted = append(uncommitted[0:n], uncommitted[n+1:]...)
							chain.Push(&blockdb.IndexedBlock{
								Epoch: block.Epoch,
								Data:  block.Serialize(),
								Items: socialCommitToIndex(block, idx),
							}, block.Epoch)
							break
						} else {
							slog.Warn("Social Indexer Provider: block seal hash mismatch")
						}
					}
				}
			}
		}
	}()
	return chain

}

func BreezeIndexerProvider(ctx context.Context, standby *swell.StandByNode, idx func([]byte) []crypto.Hash) *util.Chain[*blockdb.IndexedBlock] {
	epoch := standby.Blockchain.LastCommitEpoch
	chain := util.NewChain[*blockdb.IndexedBlock](ctx, epoch)
	go func() {
		for {
			if !standby.LastEvents.Wait() {
				slog.Warn("Breeze Indexer Provider: standby node terminated")
				return
			}
			recentCommit := standby.Blockchain.RecentAfter(epoch)
			if len(recentCommit) > 0 {
				for _, block := range recentCommit {
					indexed := &blockdb.IndexedBlock{
						Epoch: block.Header.Epoch,
						Data:  block.Serialize(),
						Items: breezeCommitToIndex(block, idx),
					}
					chain.Push(indexed, block.Header.Epoch)
				}
			}
		}
	}()
	return chain
}

func breezeIndexFn(data []byte) []crypto.Hash {
	tokens := actions.GetTokens(data)
	hashes := make([]crypto.Hash, 0, len(tokens))
	for _, token := range tokens {
		hashes = append(hashes, crypto.HashToken(token))
	}
	return hashes
}

func BreezeStandardProvider(ctx context.Context, standby *swell.StandByNode) *util.Chain[*blockdb.IndexedBlock] {
	return BreezeIndexerProvider(ctx, standby, breezeIndexFn)
}

func breezeCommitToIndex(commit *chain.CommitBlock, indexFn func([]byte) []crypto.Hash) []blockdb.IndexItem {
	indexed := make([]blockdb.IndexItem, 0)
	header := commit.Header.Serialize()
	invalidated := make(map[crypto.Hash]struct{})
	for _, hash := range commit.Commit.Invalidated {
		invalidated[hash] = struct{}{}
	}
	offset := len(header) + 4 // header + actions length
	for n := 0; n < commit.Actions.Len(); n++ {
		action := commit.Actions.Get(n)
		hash := crypto.Hasher(action)
		if _, ok := invalidated[hash]; !ok {
			hashes := indexFn(action)
			for _, hash := range hashes {
				indexed = append(indexed, blockdb.IndexItem{Hash: hash, Offset: offset})
			}
		}
		offset += len(action) + 2 // action bytes + action length
	}
	return indexed
}

func socialCommitToIndex(block *social.SocialBlock, indexFn func([]byte) []crypto.Hash) []blockdb.IndexItem {
	indexed := make([]blockdb.IndexItem, 0)
	header := block.Header()
	invalidated := make(map[crypto.Hash]struct{})
	for _, hash := range block.Invalidated {
		invalidated[hash] = struct{}{}
	}
	offset := len(header) + 2
	for n := 0; n < block.Actions.Len(); n++ {
		action := block.Actions.Get(n)
		hash := crypto.Hasher(action)
		if _, ok := invalidated[hash]; !ok {
			hashes := indexFn(action)
			for _, hash := range hashes {
				indexed = append(indexed, blockdb.IndexItem{Hash: hash, Offset: offset})
			}
		}
		offset += len(action) + 2 // action bytes + action length
	}
	return indexed

}
