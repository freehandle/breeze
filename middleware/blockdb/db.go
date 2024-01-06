package blockdb

import (
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

func NewBlockchainHistory(path string) (*BlockchainHistory, error) {
	blockstore, err := NewBlockStore(path)
	if err != nil {
		return nil, err
	}
	indexed, err := NewIndexed(path)
	if err != nil {
		return nil, err
	}
	return &BlockchainHistory{
		Storage: blockstore,
		Index:   indexed,
	}, nil
}

type BlockchainHistory struct {
	Storage *BlockStore
	Index   *Indexed
}

func (b *BlockchainHistory) Incorporate(block *chain.CommitBlock) error {
	b.Index.IndexCommit(block)
	return b.Storage.AppendBlock(block.Serialize(), int64(block.Header.Epoch))
}

func (b *BlockchainHistory) Retrieve(epoch int64, offset int64) []byte {
	if b.Storage == nil {
		return nil
	}
	return b.Storage.GetBlock(epoch)
}

func (b *BlockchainHistory) Find(token crypto.Token) [][]byte {
	if b.Index == nil {
		return nil
	}
	found := make([][]byte, 0)
	indexed := b.Index.Search(token)
	for _, index := range indexed {
		data := b.Storage.GetItem(index.Height, index.Offset)
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
