package index

import (
	"errors"
	"log/slog"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/papirus"
)

/*const (
	IndexSize = 8
	ItemBytes = IndexSize + 8
)*/

type IndexPosition struct {
	Epoch  uint64
	Offset int64
}

func compare(hash crypto.Hash, data []byte, IndexSize int) bool {
	if len(data) < IndexSize {
		return false
	}
	for n := 0; n < IndexSize; n++ {
		if hash[n] != data[n] {
			return false
		}
	}
	return true
}

//type IndexToken [IndexSize]byte

type Index struct {
	indexSize      int
	itemBytes      int
	store          *papirus.BucketStore
	bitsForBucket  int
	lastBucket     []int64
	itemsCount     []int64
	itemsPerBucket int64
}

func (i *Index) Close() {
	i.store.Close()
}

func (i *Index) Get(hash crypto.Hash, starting uint64) []IndexPosition {
	found := make([]IndexPosition, 0)
	initialBucket := roundIndexToken(hash, i.bitsForBucket)
	if initialBucket < 0 || initialBucket >= int64(len(i.lastBucket)) {
		slog.Error("rounIndexToken out of range")
		return found
	}
	bucket := i.store.ReadBucket(initialBucket)
	if bucket == nil {
		slog.Error("could not read bucket")
		return found
	}
	for {
		for n := int64(0); n < i.itemsPerBucket; n++ {
			item := bucket.ReadItem(n)
			if compare(hash, item, i.indexSize) {
				newItem := IndexPosition{
					Epoch:  uint64(item[i.indexSize]) + uint64(item[i.indexSize+1])<<8 + uint64(item[i.indexSize+2])<<16 + uint64(item[i.indexSize+3])<<24,
					Offset: int64(item[i.indexSize+4]) + int64(item[i.indexSize+5])<<8 + int64(item[i.indexSize+6])<<16 + int64(item[i.indexSize+7])<<24,
				}
				if newItem.Epoch >= starting {
					found = append(found, newItem)
				}
			}
		}
		bucket = bucket.NextBucket()
		if bucket == nil {
			return found
		}
	}
}

func (i *Index) Append(hash crypto.Hash, epoch uint64, offset int) {
	data := append(hash[:i.indexSize], byte(epoch), byte(epoch>>8), byte(epoch>>16), byte(epoch>>24), byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24))
	bucket := roundIndexToken(hash, i.bitsForBucket)
	lastBucket := i.lastBucket[bucket]
	itemPosition := (i.itemsCount[bucket] + 1) % i.itemsPerBucket
	i.store.WriteAt(lastBucket, itemPosition, data)
	i.itemsCount[bucket] += 1
	if itemPosition == i.itemsPerBucket-1 {
		b := i.store.ReadBucket(lastBucket)
		nextBucket := b.AppendOverflow()
		i.lastBucket[bucket] = nextBucket.N
	}
}

func roundIndexToken(hash crypto.Hash, bits int) int64 {
	numeric := int64(hash[0]) + int64(hash[1])<<8 + int64(hash[2])<<16 + int64(hash[3])<<24
	return numeric - (numeric>>bits)<<bits
}

//func roundUint64(value uint64, bits int) uint64 {
//	return value - (value>>bits)<<bits
//}

func NewIndex(indexPath string, bitsForBucket, itemsPerBucket, indexSize int64) (*Index, error) {
	itemBytes := indexSize + 8
	if bitsForBucket < 1 || bitsForBucket > 32 {
		return nil, errors.New("bitsForBucket must be between 1 and 32")
	}
	initialBuckets := int64(1 << bitsForBucket)
	initialSize := initialBuckets*(itemsPerBucket*itemBytes+8) + papirus.HeaderSize

	var bs papirus.ByteStore
	if indexPath == "" {
		bs = papirus.NewMemoryStore(initialSize)
	} else {
		if fs := papirus.NewFileStore(indexPath, initialSize); fs == nil {
			return nil, errors.New("could not create index file")
		} else {
			bs = fs
		}
	}
	store := papirus.NewBucketStore(itemBytes, itemsPerBucket, bs)
	if store == nil {
		return nil, errors.New("could not create index bucket store")
	}
	index := &Index{
		indexSize:      int(indexSize),
		store:          store,
		bitsForBucket:  int(bitsForBucket),
		lastBucket:     make([]int64, initialBuckets),
		itemsCount:     make([]int64, initialBuckets),
		itemsPerBucket: itemsPerBucket,
	}
	for n := int64(0); n < initialBuckets; n++ {
		index.lastBucket[n] = n
	}
	return index, nil
}

func OpenIndex(indexPath string, bitsForBucket, itemsPerBucket, indexSize int64) (*Index, error) {
	itemBytes := indexSize + 8
	if indexPath == "" {
		return nil, errors.New("indexPath cannot be empty")
	}
	bs := papirus.OpenFileStore(indexPath)
	if bs == nil {
		return nil, errors.New("could not open index file")
	}
	store := papirus.NewBucketStore(itemBytes, itemsPerBucket, bs)
	if store == nil {
		return nil, errors.New("could not open index bucket store")
	}
	initialBuckets := int64(1 << bitsForBucket)
	index := &Index{
		indexSize:      int(indexSize),
		store:          store,
		bitsForBucket:  int(bitsForBucket),
		lastBucket:     make([]int64, initialBuckets),
		itemsCount:     make([]int64, initialBuckets),
		itemsPerBucket: itemsPerBucket,
	}
	for n := 0; n < int(initialBuckets); n++ {
		b := index.store.ReadBucket(int64(n))
		count := int64(1)
		for {
			item := b.NextBucket()
			if item == nil {
				break
			}
			count += 1
			b = item
		}
		index.lastBucket[n] = b.N
		isFullBucket := true
		for j := int64(0); j < itemsPerBucket; j++ {
			item := b.ReadItem(j)
			isNull := true
			for k := 0; k < index.indexSize; k++ {
				if item[k] != 0 {
					isNull = false
					break
				}
			}
			if isNull {
				index.itemsCount[n] = (count-1)*itemsPerBucket + j
				isFullBucket = false
				break
			}
		}
		if isFullBucket {
			bs.Close()
			return nil, errors.New("index file corrupt: found full bucket without overflow pointer")
		}
	}
	return index, nil
}
