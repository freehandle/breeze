package index

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
	"github.com/freehandle/cb/blocks"
	"github.com/freehandle/papirus"
)

type Indexer func([]byte) []crypto.Hash

const (
	StoreBytes = 16
	IndexBytes = 4
	NStores    = 1
)

type Index struct {
	store     [NStores]papirus.ByteStore
	index     [NStores]map[[IndexBytes]byte][]int64
	lastEpoch uint64
}

func (i *Index) LastIndexedEpoch() uint64 {
	return i.lastEpoch
}

func (i *Index) NextBlock(epoch uint64) {
	i.lastEpoch = epoch
	data := make([]byte, 0)
	util.PutUint64(epoch, &data)
	i.store[0].WriteAt(0, data)
}

func OpenFileStoreIndex(path [NStores]string) *Index {
	var stores [NStores]papirus.ByteStore
	for n := 0; n < NStores; n++ {
		stores[n] = papirus.OpenFileStore(path[n])
	}
	return OpenIndex(stores)
}

func OpenIndex(stores [NStores]papirus.ByteStore) *Index {
	var i Index
	i.store = stores
	for n := 0; n < NStores; n++ {
		file := stores[n]
		i.index[n] = make(map[[IndexBytes]byte][]int64)
		startPos := int64(0)
		if n == 0 {
			data := file.ReadAt(0, 8)
			i.lastEpoch, _ = util.ParseUint64(data, 0)
			startPos = 8

		}
		for position := startPos; position < file.Size(); position += StoreBytes + 8 {
			data := file.ReadAt(position, StoreBytes+8)
			var partial [IndexBytes]byte
			copy(partial[:], data[:IndexBytes])
			if indexed, ok := i.index[n][partial]; ok {
				i.index[n][partial] = append(indexed, position)
			} else {
				i.index[n][partial] = []int64{position}
			}
		}
	}
	return &i
}

func PartialHash(hash crypto.Hash) [IndexBytes]byte {
	var partial [IndexBytes]byte
	copy(partial[:], hash[:IndexBytes])
	return partial
}

func HashToFileNum(hash crypto.Hash) int {
	return 0
}

func (i *Index) Add(epoch, sequece int, hashes []crypto.Hash) {
	value := make([]byte, 0, 8)
	util.PutUint64(uint64(epoch<<32+sequece), &value)
	for _, h := range hashes {
		fileNum := HashToFileNum(h)
		file := i.store[fileNum]
		// data = partial hash and block position
		data := make([]byte, StoreBytes, StoreBytes+8)
		copy(data, h[:StoreBytes])
		data = append(data, value...)
		// save and index
		position := file.Size()
		file.Append(data)
		partial := PartialHash(h)
		if indexed, ok := i.index[fileNum][partial]; ok {
			i.index[fileNum][partial] = append(indexed, position)
		} else {
			i.index[fileNum][partial] = []int64{position}
		}
	}
}

type Positions map[uint64][]int // block and sequences

type IndexItem struct {
	PartialHash [StoreBytes]byte
	Epoch       int64
	Sequence    int64
}

type Place struct {
	Epoch    int64
	Sequence int64
}

func compareHash(hash crypto.Hash, partial []byte) bool {
	if len(partial) < StoreBytes {
		return false
	}
	for n := 0; n < StoreBytes; n++ {
		if hash[n] != partial[n] {
			return false
		}
	}
	return true
}

func (i *Index) Retrieve(hash crypto.Hash) []*blocks.QueryBlock {
	n := HashToFileNum(hash)
	partial := PartialHash(hash)
	indexed, ok := i.index[n][partial]
	if !ok {
		return nil
	}
	positions := make(map[uint64][]int)
	store := i.store[n]
	for _, idx := range indexed {
		data := store.ReadAt(idx, StoreBytes+8)
		if compareHash(hash, data) {
			value, _ := util.ParseUint64(data, StoreBytes)
			epoch := uint64(value) >> 32
			sequence := (int(value) << 32) >> 32
			if sequences, ok := positions[epoch]; !ok {
				positions[epoch] = append(sequences, sequence)
			} else {
				positions[epoch] = []int{sequence}
			}
		}
	}
	output := make([]*blocks.QueryBlock, len(positions))
	for epoch, actions := range positions {
		output = append(output, &blocks.QueryBlock{Epoch: epoch, Actions: actions})
	}
	return output
}
