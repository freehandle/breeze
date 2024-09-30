package util

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const MAX_FILE_SIZE = 1 << 30

type ReadAppendMutiFileStore struct {
	name        string
	path        string
	mu          sync.Mutex
	totalsize   int64
	filesizes   []int64
	files       []*os.File
	writeOffset int64
	readOffset  int64
	readCurrent int
}

type plan struct {
	file   int
	offset int64
	nbytes int64
	total  int64
}

func (r *ReadAppendMutiFileStore) prepareReadPlan(nbytes int64) []plan {
	r.mu.Lock()
	defer r.mu.Unlock()
	remaining := nbytes
	file := r.readCurrent
	offset := r.readOffset
	strategy := make([]plan, 0)
	for {
		if remaining <= r.filesizes[file]-offset {
			p := plan{file, offset, remaining, nbytes}
			strategy = append(strategy, p)
			return strategy
		} else {
			bytesToRead := r.filesizes[file] - offset
			p := plan{file, offset, bytesToRead, nbytes - remaining + bytesToRead}
			strategy = append(strategy, p)
			remaining -= bytesToRead
			if file == len(r.files)-1 {
				return strategy
			}
			file++
			offset = 0
		}
	}
}

func (r *ReadAppendMutiFileStore) Read(p []byte) (int, error) {
	plan := r.prepareReadPlan(int64(len(p)))
	for _, plan := range plan {
		_, err := r.files[plan.file].ReadAt(p[plan.total-plan.nbytes:plan.total], plan.offset)
		if err != nil && err != io.EOF {
			return 0, err
		}
	}
	last := plan[len(plan)-1]
	r.readCurrent = last.file
	r.readOffset = last.offset + last.nbytes
	total := last.total
	if total < int64(len(p)) {
		return int(total), io.EOF
	}
	return int(total), nil
}

func (r *ReadAppendMutiFileStore) createfile() error {
	file, err := os.OpenFile(fmt.Sprintf("%s%d.bin", r.name, len(r.files)), os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	r.files = append(r.files, file)
	r.filesizes = append(r.filesizes, 0)
	r.writeOffset = 0
	return nil
}

func (r *ReadAppendMutiFileStore) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	file := r.files[len(r.files)-1]
	n, err := file.WriteAt(p, r.writeOffset)
	if err != nil {
		return n, err
	}
	wbytes := int64(n)
	r.writeOffset += wbytes
	r.totalsize += wbytes
	if r.writeOffset >= MAX_FILE_SIZE {
		r.createfile()
	}
	return n, err
}

func (r *ReadAppendMutiFileStore) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, file := range r.files {
		if file != nil {
			file.Close()
		}
	}
	return nil
}

func OpenMultiFileStore(dbpath, name string) (*ReadAppendMutiFileStore, error) {
	dirfiles, err := os.ReadDir(dbpath)
	if err != nil {
		log.Fatal(err)
	}
	numbers := make(sort.IntSlice, 0)
	stats := make(map[int]int64)
	totalSize := int64(0)
	for _, file := range dirfiles {
		if strings.HasPrefix(file.Name(), name) && strings.HasSuffix(file.Name(), ".bin") {
			if number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(file.Name(), name), ".bin")); err != nil {
				numbers = append(numbers, number)
				if info, err := file.Info(); err != nil {
					return nil, err
				} else {
					size := info.Size()
					stats[number] = size
					totalSize += size
				}
			}
		}
	}
	if len(numbers) == 0 {
		filename := path.Join(dbpath, fmt.Sprintf("%s0.bin", name))
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			return nil, err
		}
		store := &ReadAppendMutiFileStore{
			mu:        sync.Mutex{},
			name:      name,
			path:      dbpath,
			filesizes: []int64{0},
			files:     []*os.File{file},
		}
		return store, nil
	}

	sort.Sort(numbers)
	for n := 0; n < len(numbers); n++ {
		if numbers[n] != n {
			return nil, fmt.Errorf("missing file %s%d.bin", name, n)
		}
	}

	store := &ReadAppendMutiFileStore{
		mu:          sync.Mutex{},
		name:        name,
		path:        dbpath,
		totalsize:   totalSize,
		filesizes:   make([]int64, len(numbers)),
		files:       make([]*os.File, len(numbers)),
		writeOffset: stats[len(numbers)-1],
	}

	for n := 0; n < len(numbers); n++ {
		filename := path.Join(dbpath, fmt.Sprintf("%s%d.bin", name, n))
		store.files[n], err = os.Open(filename)
		if err != nil {
			store.Close()
			return nil, err
		}
		store.filesizes[n] = stats[n]
	}
	return store, nil
}
