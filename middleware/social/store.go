package social

import (
	"errors"

	"github.com/freehandle/breeze/util"
	"github.com/freehandle/papirus"
)

type BlockIndex struct {
	StoreNum int
	Offset   int64
	Size     int64
}

type BlockStore struct {
	stores      []papirus.ByteStore
	maxSize     int64
	blocks      []BlockIndex
	currentSize int64
	Epoch       uint64
}

func NewBlockStore(store papirus.ByteStore) *BlockStore {
	return &BlockStore{
		stores:  []papirus.ByteStore{store},
		maxSize: store.Size(),
		blocks:  make([]BlockIndex, 0),
	}
}

func OpenBlockStore(stores []papirus.ByteStore, maxSize int64) *BlockStore {
	blocks := make([]BlockIndex, 0)
	epoch := -1
	var offset int64
	for n, store := range stores {
		offset = int64(0)
		for {
			sizeBytes := store.ReadAt(offset, 8)
			size, _ := util.ParseUint64(sizeBytes, 0)
			if size == 0 {
				break
			}
			epoch += 1
			index := BlockIndex{
				StoreNum: n,
				Offset:   offset + 8,
				Size:     int64(size),
			}
			blocks = append(blocks, index)
			offset = offset + 8 + int64(size)
			if offset >= store.Size() {
				break
			}
		}
	}
	return &BlockStore{
		stores:      stores,
		maxSize:     maxSize,
		blocks:      blocks,
		currentSize: offset,
		Epoch:       uint64(epoch),
	}
}

func (b *BlockStore) NewStore() {
	current := b.stores[len(b.stores)-1]
	b.stores = append(b.stores, current.New(b.maxSize))
	b.currentSize = 0
}

func (b *BlockStore) AddBlock(data []byte) error {
	if len(b.stores) == 0 {
		return errors.New("no data storage specified")
	}
	bytes := make([]byte, 0)
	util.PutUint64(uint64(len(data)), &bytes)
	data = append(bytes, data...)

	if b.currentSize+int64(len(data)) > b.maxSize {
		b.NewStore()
	}
	index := BlockIndex{
		StoreNum: len(b.stores) - 1,
		Offset:   b.currentSize + 8,
		Size:     int64(len(data)) - 8,
	}
	b.blocks = append(b.blocks, index)
	current := b.stores[len(b.stores)-1]
	current.WriteAt(b.currentSize, data)
	b.currentSize += int64(len(data))
	b.Epoch += 1
	return nil
}

func (b *BlockStore) GetBlock(n int) []byte {
	if n < 0 || n >= len(b.blocks) {
		return nil
	}
	index := b.blocks[n]
	store := b.stores[index.StoreNum]
	return store.ReadAt(index.Offset, index.Size)
}
