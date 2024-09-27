package util

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const MAX_FILE_SIZE = 1 << 30

type ReadAppendMutiFileStore struct {
	mu sync.RWMutex
	totalsize int64
	filesizes []int64
	files     []*os.File
	readOffset  int
	readCurrent int
}

func (r *ReadAppendMutiFileStore) ReadFromFile(offset, nbytes int64, file int) ([]bytes, error) {
	r.mu.RLock()
	if file == len(r.files) - 1 {
		bytes := make([]byte, nbytes)
		rbytes, err := r.files[file].ReadAt(bytes, offset)
		r.mu.Unlock()
		if rbytes < int(nbytes) {
			return bytes[:rbytes], io.EOF
		}
		return bytes[:rbytes], nil
	}
	r.mu.RUnlock()
	bytes := make([]byte, nbytes)
	nbytes, err := r.files[len(r.files)-1].ReadAt(bytes, offset)
	return bytes[:nbytes], nil
}

func (r *ReadAppendMutiFileStore) Write(p []byte) (n int, err error) {
	r.mu.WLock()
	if r.readOffset + len(p) >= r.filesizes[r.readCurrent] {
		r.readOffset = 0
		r.readCurrent++
	}
	n, err = r.files[r.readCurrent].WriteAt(p, r.readOffset)
	r.readOffset += n
	r.totalsize += int64(n)
	r.mu.WUnlock()
	return n, err
}

func (r *ReadAppendMutiFileStore) Read(p []byte) (n int, err error) {
	
	currentSize = r.filesizes[r.current]
	if r.readOffset + len(p) >= currentSize {

	}

	
	
	n, err = r.files[r.current].Read(p)
	if

func OpenMultiFileStore(path, name string) (*MutiFileStore, error) {
	dirfiles, err := os.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}
	numbers := make(sort.IntSlice, 0)
	for _, file := range dirfiles {
		if strings.HasPrefix(file.Name(), name) && strings.HasSuffix(file.Name(), ".bin") {
			if number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(file.Name(), name), ".bin")); err != nil {
				numbers = append(numbers, number)
			}
		}
	}
	sort.Sort(numbers)
	for n := 0; n < len(numbers); n++ {
		if numbers[n] != n {
			return nil, fmt.Errorf("missing file %s%d.bin", name, n)
		}
	}
	openfiles := make([]*os.File, len(numbers))
	for n := 0; n < len(numbers)-1; n++ {
		filePath := fmt.Sprintf("%s/%s%d.bin", path, name, n)
		openfiles[n], err = os.Open(filePath)
		if err != nil {
			return nil, err
		}
	}
	openfiles[n], err = os.Open(filePath)

}
