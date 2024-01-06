package blockdb

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/freehandle/breeze/util"
	"github.com/freehandle/papirus"
)

const Age = 7 * 24 * 60 * 60 // One week
const prefix = "blockage_"

type BlockStore struct {
	Ages       []papirus.ByteStore
	Offsets    [][Age]int64 // start of each block within age
	LastCommit int64
	path       string
}

func lastOffset(offsets [Age]int64) int {
	for n := 0; n < len(offsets); n++ {
		if offsets[n] == 0 {
			return n
		}
	}
	return Age
}

func OpenAge(filePath string) (papirus.ByteStore, [Age]int64, error) {
	var offsets [Age]int64
	store := papirus.OpenFileStore(filePath)
	if store == nil {
		return nil, offsets, fmt.Errorf("could not open filestore %s", filePath)
	}
	if store.Size() == 0 {
		return store, offsets, nil
	}
	position := int64(0)
	count := 0
	for {
		sizeBytes := store.ReadAt(position, 8)
		size, _ := util.ParseUint64(sizeBytes, 0)
		if size == 0 {
			return nil, offsets, fmt.Errorf("invalid block size at position %d", position)
		}
		size64 := int64(size)
		position += 8
		if position+size64 > store.Size() {
			return nil, offsets, fmt.Errorf("invalid block size at position %d", position)
		}
		if position+size64+8 == store.Size() {
			return store, offsets, nil
		}
		position += size64
		count += 1
		offsets[count] = position
	}
}

func NewBlockStore(path string) (*BlockStore, error) {
	if path == "" {
		return &BlockStore{
			Ages:    []papirus.ByteStore{papirus.NewMemoryStore(0)},
			Offsets: make([][Age]int64, 1),
			path:    path,
		}, nil
	}
	filePath := filepath.Join(path, "blocks_0")
	store := papirus.NewFileStore(filePath, 0)
	if store != nil {
		return nil, fmt.Errorf("could not open filestore %s", filePath)
	}
	return &BlockStore{
		Ages:    []papirus.ByteStore{store},
		Offsets: make([][Age]int64, 1),
		path:    path,
	}, nil
}

func OpenBlockStore(path string) (*BlockStore, error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	values := make([]int, 0)
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasPrefix(file.Name(), "blocks_") {
			continue
		}
		count := strings.Replace(file.Name(), "blocks_", "", 1)
		value, err := strconv.Atoi(count)
		if err != nil {
			continue
		}
		values = append(values, value)
	}
	sort.Ints(values)
	if len(values) == 0 {
		return NewBlockStore(path)
	}
	for n := 0; n < len(values); n++ {
		if values[n] != n {
			return nil, fmt.Errorf("missing age %v from block store", n)
		}
	}
	store := BlockStore{
		Ages:    make([]papirus.ByteStore, len(values)),
		Offsets: make([][Age]int64, len(values)),
		path:    path,
	}
	for n := 0; n < len(values); n++ {
		var err error
		store.Ages[n], store.Offsets[n], err = OpenAge(filepath.Join(path, fmt.Sprintf("blocks_%d.dat", n)))
		if err != nil {
			for m := 0; m < n; m++ {
				store.Ages[m].Close()
			}
			return nil, err
		}
		if (n != len(values)-1) && lastOffset(store.Offsets[n]) != Age {
			for m := 0; m < n; m++ {
				store.Ages[m].Close()
			}
			return nil, fmt.Errorf("block in age %d uncomplete", n)
		}

		if n != len(values)-1 {
			store.LastCommit = int64(n*Age + lastOffset(store.Offsets[n]))
		}
	}
	return &store, nil
}

func (b *BlockStore) AppendBlock(data []byte, epoch int64) error {
	if epoch != b.LastCommit+1 {
		return fmt.Errorf("block epoch %d is not sequential to last commit %d", epoch, b.LastCommit)
	}
	// Age 1 epoch 1 to 900
	// Age 2 epoch 901 to 1800
	if epoch%Age == 1 {
		b.Offsets = append(b.Offsets, [Age]int64{})
		if b.path == "" {
			b.Ages = append(b.Ages, papirus.NewMemoryStore(0))
		} else {
			filePatg := filepath.Join(b.path, fmt.Sprintf("blocks_%d.dat", len(b.Ages)))
			store := papirus.NewFileStore(filePatg, 0)
			if store == nil {
				return fmt.Errorf("could not open new filestore %s", filePatg)
			}
			b.Ages = append(b.Ages, store)
		}
	}
	fileCount := (epoch - 1) / Age
	store := b.Ages[fileCount]

	bytes := util.Uint64ToBytes(uint64(len(data)))
	data = append(bytes, data...)
	store.Append(data)
	b.LastCommit = epoch
	return nil
}
