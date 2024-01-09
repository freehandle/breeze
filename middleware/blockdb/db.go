package blockdb

import (
	"path/filepath"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/blockdb/index"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/util"
)

const ItemsPerBucket = 8
const BitsForBucket = 20

func NewBlockchainHistory(path string, iindex bool) (*BlockchainHistory, error) {
	blockstore, err := NewBlockStore(path)
	if err != nil {
		return nil, err
	}
	if !iindex {
		return &BlockchainHistory{
			Storage: blockstore,
		}, nil

	}
	indexPath := filepath.Join(path, "block_index")
	indx, err := index.NewIndex(indexPath, BitsForBucket, ItemsPerBucket)
	if err != nil {
		return nil, err
	}
	return &BlockchainHistory{
		Storage: blockstore,
		Index:   indx,
	}, nil
}

func OpenBlockchainHistory(path string, indexed bool) (*BlockchainHistory, error) {
	blockstore, err := OpenBlockStore(path)
	if err != nil {
		return nil, err
	}
	if !indexed {
		return &BlockchainHistory{
			Storage: blockstore,
		}, nil
	}
	indexPath := filepath.Join(path, "block_index")
	indx, err := index.OpenIndex(indexPath, BitsForBucket, ItemsPerBucket)
	if err != nil {
		return nil, err
	}
	return &BlockchainHistory{
		Storage: blockstore,
		Index:   indx,
	}, nil
}

type BlockchainHistory struct {
	Storage *BlockStore
	Index   *index.Index
}

func (b *BlockchainHistory) Incorporate(commit *chain.CommitBlock) error {
	err := b.Storage.AppendBlock(commit.Serialize(), int64(commit.Header.Epoch))
	if err != nil {
		return err
	}
	if b.Index == nil {
		return nil
	}
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
			tokens := actions.GetTokens(action)
			for _, token := range tokens {
				hashToken := crypto.HashToken(token)
				b.Index.Append(hashToken, commit.Header.Epoch, offset)
			}
		}
		offset += len(action) + 2 // action bytes + action length
	}
	return nil
}

func (b *BlockchainHistory) Retrieve(epoch int64, offset int64) []byte {
	if b.Storage == nil {
		return nil
	}
	return b.Storage.GetBlock(epoch)
}

func (b *BlockchainHistory) Find(hash crypto.Hash, startingAt uint64) [][]byte {
	if b.Index == nil {
		return nil
	}
	indexed := b.Index.Get(hash, startingAt)
	found := make([][]byte, 0)
	for _, index := range indexed {
		data := b.Storage.GetItem(int64(index.Epoch), index.Offset)
		if len(data) > 0 {
			found = append(found, data)
		}
	}
	return found
}

func (b *BlockStore) GetItem(epoch int64, offset int64) []byte {
	if epoch > b.LastCommit {
		return nil
	}
	age := (epoch - 1/Age)
	if age >= int64(len(b.Ages)) {
		return nil
	}
	blockOffset := b.Offsets[age][(epoch-1)%Age]
	bytes := b.Ages[age].ReadAt(blockOffset+8+offset, 2)
	size, _ := util.ParseUint16(bytes, 0)
	return b.Ages[age].ReadAt(blockOffset+8+offset+2, int64(size))
}

func (b *BlockStore) GetBlock(epoch int64) []byte {
	if epoch > b.LastCommit {
		return nil
	}
	age := (epoch - 1) / Age
	if age >= int64(len(b.Ages)) {
		return nil
	}
	blockOffset := b.Offsets[age][(epoch-1)%Age]
	bytes := b.Ages[age].ReadAt(blockOffset, 8)
	size, _ := util.ParseUint64(bytes, 0)
	return b.Ages[age].ReadAt(blockOffset+8, int64(size))
}
