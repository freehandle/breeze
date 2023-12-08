package chain

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

// Memory block is a simplified version of block containing only actions bytes
// stored in a sequential byte slice.
type ActionArray struct {
	actions []int // end of n-th instruction
	data    []byte
}

func NewActionArray() *ActionArray {
	return &ActionArray{
		actions: make([]int, 0),
		data:    make([]byte, 0),
	}
}

func ParseAction(data []byte, position int) (*ActionArray, int) {
	actions, position := util.ParseActionsArray(data, position)
	if len(actions) == 0 {
		return &ActionArray{
			actions: make([]int, 0),
			data:    make([]byte, 0),
		}, position
	}
	actionArray := &ActionArray{
		actions: make([]int, 0, len(actions)),
		data:    make([]byte, 0),
	}
	for _, action := range actions {
		actionArray.Append(action)
	}
	return actionArray, position
}

func (b *ActionArray) Serialize() []byte {
	actions := make([][]byte, len(b.actions))
	for n := 0; n < len(b.actions); n++ {
		actions[n] = b.Get(n)
	}
	bytes := make([]byte, 0)
	util.PutActionsArray(actions, &bytes)
	return bytes
}

func (b *ActionArray) Hash() crypto.Hash {
	if len(b.actions) == 0 {
		return crypto.ZeroValueHash
	}
	return crypto.Hasher(b.data)
}

func (b *ActionArray) Len() int {
	return len(b.actions)
}

func (b *ActionArray) Get(n int) []byte {
	if n >= len(b.actions) || n < 0 {
		return nil
	}
	starts := 0
	if n > 0 {
		starts = b.actions[n-1]
	}
	ends := b.actions[n]
	return b.data[starts:ends]
}

func (b *ActionArray) Append(data []byte) {
	b.data = append(b.data, data...)
	b.actions = append(b.actions, len(b.data))
}

func (m *ActionArray) Clone() *ActionArray {
	data := make([]byte, len(m.data))
	copy(data, m.data)
	actions := make([]int, len(m.actions))
	copy(actions, m.actions)
	return &ActionArray{
		actions: actions,
		data:    data,
	}
}
