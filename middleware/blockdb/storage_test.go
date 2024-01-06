package blockdb

import (
	"testing"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

func TestStorage(t *testing.T) {
	store, err := NewBlockStore("")
	if err != nil {
		t.Fatal(err)
	}

	for n := 1; n < 100; n++ {
		hash := crypto.Hasher(util.Uint64ToBytes(uint64(n)))
		err := store.AppendBlock(hash[:], int64(n))
		if err != nil {
			t.Fatal(err)
		}
	}
	for n := 1; n < 100; n++ {
		bytes := store.GetBlock(int64(n))
		if len(bytes) != crypto.Size {
			t.Fatalf("invalid block size: %v instead of %v", len(bytes), crypto.Size)
		}
	}

}
