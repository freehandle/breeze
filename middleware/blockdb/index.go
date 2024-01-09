package blockdb

const FileNum = 1
const IndexTokenSize = 8
const EntrySize = IndexTokenSize + 8

/*type Indexed struct {
	files [FileNum]papirus.ByteStore
}

func OpenIndexed(path string) (*Indexed, error) {
	if path == "" {
		return NewIndexed("")
	}
	var i Indexed
	for n := 0; n < FileNum; n++ {
		filePath := filepath.Join(path, fmt.Sprintf("indexed_%d.dat", n))
		if store := papirus.NewFileStore(filePath, 0); store == nil {
			return nil, fmt.Errorf("could not open filestore %s", filePath)
		} else {
			i.files[n] = store
		}
	}
	return &i, nil
}

func NewIndexed(path string) (*Indexed, error) {
	var i Indexed
	if path == "" {
		for n := 0; n < FileNum; n++ {
			i.files[n] = papirus.NewMemoryStore(0)
		}
		return &i, nil
	}
	for n := 0; n < FileNum; n++ {
		filePath := filepath.Join(path, fmt.Sprintf("indexed_%d.dat", n))
		if store := papirus.NewFileStore(filePath, 0); store == nil {
			return nil, fmt.Errorf("could not open filestore %s", filePath)
		} else {
			i.files[n] = store
		}
	}
	return &i, nil
}

type IndexOffset struct {
	IndexToken IndexToken
	Offset     uint64
}

type IndexOffsets []IndexOffset

func (i IndexOffsets) Serialize() []byte {
	bytes := make([]byte, 0)
	for _, index := range i {
		bytes = append(bytes, index.IndexToken[:]...)
		util.PutUint64(index.Offset, &bytes)
	}
	return bytes
}

func (i IndexOffsets) Len() int {
	return len(i)
}

func (i IndexOffsets) Less(a, b int) bool {
	for n := 0; n < IndexTokenSize; n++ {
		if i[a].IndexToken[n] < i[b].IndexToken[n] {
			return true
		}
	}
	return i[a].Offset < i[b].Offset
}

func (i IndexOffsets) Swap(a, b int) {
	i[a], i[b] = i[b], i[a]
}

type IndexToken [IndexTokenSize]byte

func TokenToIndexToken(token crypto.Token) IndexToken {
	var index IndexToken
	copy(index[:], token[:IndexTokenSize])
	return index
}

type ItemPosition struct {
	Height int64
	Offset int64
}

func (i *Indexed) GetFile(token crypto.Token) papirus.ByteStore {
	return i.files[0]
}

func GetFileNum(token crypto.Token) int {
	return 0
}

func Compare(data []byte, offset int64, token crypto.Token) bool {
	for n := int64(0); n < IndexTokenSize; n++ {
		if data[offset+n] != token[n] {
			return false
		}
	}
	return true
}

func (i *Indexed) Search(token crypto.Token) []ItemPosition {
	file := i.GetFile(token)
	fileSize := file.Size()
	position := int64(0)
	found := make([]ItemPosition, 0)
	epoch := int64(0)
	for {
		blockIndexSizeBytes := file.ReadAt(position, 4)
		size, _ := util.ParseUint32(blockIndexSizeBytes, 0)
		position += 4
		bytes := file.ReadAt(position, int64(size))
		for {
			if Compare(bytes, position, token) {
				// TODO: int vs int64??
				offset, _ := util.ParseUint64(bytes, int(position)+IndexTokenSize)
				found = append(found, ItemPosition{Height: epoch, Offset: int64(offset)})
			}
			position += EntrySize + 8
			if position >= fileSize {
				break
			}
		}
		if position >= fileSize {
			break
		}
		epoch += 1
	}
	return found
}

func (i *Indexed) IndexCommit(commit *chain.CommitBlock) {
	index := IndexCommitBlock(commit)
	sorted := IndexToSortArray(index)
	for fileNum := 0; fileNum < FileNum; fileNum++ {
		bytes := sorted[fileNum].Serialize()
		i.files[fileNum].Append(bytes)
	}
}

func IndexToSortArray(index [FileNum]map[IndexToken][]int64) [FileNum]IndexOffsets {
	var array [FileNum]IndexOffsets
	for fileNum := 0; fileNum < FileNum; fileNum++ {
		array[fileNum] = make(IndexOffsets, 0)
		for token, offsets := range index[fileNum] {
			for _, offset := range offsets {
				idxOffset := IndexOffset{IndexToken: token, Offset: uint64(offset)}
				array[fileNum] = append(array[fileNum], idxOffset)
			}
		}
		sort.Sort(array[fileNum])
	}
	return array
}

func IndexCommitBlock(commit *chain.CommitBlock) [FileNum]map[IndexToken][]int64 {
	header := commit.Header.Serialize()
	offsetHeader := int64(len(header) + 4) // header + action length (32-bit)
	invalidated := make(map[crypto.Hash]struct{})
	for _, hash := range commit.Commit.Invalidated {
		invalidated[hash] = struct{}{}
	}
	var index [FileNum]map[IndexToken][]int64
	for n := 0; n < FileNum; n++ {
		index[n] = make(map[IndexToken][]int64)
	}
	offset := int64(0)
	for n := 0; n < commit.Actions.Len(); n++ {
		action := commit.Actions.Get(n)
		hash := crypto.Hasher(action)
		if _, ok := invalidated[hash]; !ok {
			tokens := actions.GetTokens(action)
			for _, token := range tokens {
				indexToken := TokenToIndexToken(token)
				fileNum := GetFileNum(token)
				if offets, ok := index[fileNum][indexToken]; ok {
					index[fileNum][indexToken] = append(offets, offsetHeader+offset)
				} else {
					index[fileNum][indexToken] = []int64{offsetHeader + offset}
				}
			}
		}
		offset += int64(len(action)) + 2
	}
	return index
}
*/
