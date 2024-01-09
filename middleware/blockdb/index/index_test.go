package index

import (
	"testing"

	"github.com/freehandle/breeze/crypto"
)

const testIndexSize = 8

type testIndexToken [testIndexSize]byte

func TestIndex(t *testing.T) {
	idx, err := NewIndex("testdata", 8, 64, 8)
	if err != nil {
		t.Fatal(err)
	}
	if idx == nil {
		t.Fatal("could not create index")
	}
	replica := make(map[testIndexToken][]IndexPosition)
	hashes := make([]crypto.Hash, 10000)
	for n := int64(0); n < 10000; n++ {
		item := IndexPosition{
			Epoch:  uint64(n),
			Offset: 10000 + n,
		}
		hash := crypto.Hasher([]byte{byte(n), byte(n << 8)})
		hashes[n] = hash
		var i testIndexToken
		copy(i[:], hash[0:testIndexSize])
		idx.Append(hash, item.Epoch, int(item.Offset))
		replica[i] = append(replica[i], item)
	}
	for n := int64(0); n < 10000; n++ {
		idxItems := idx.Get(hashes[n], 0)
		it := testIndexToken(hashes[n][0:testIndexSize])
		if len(idxItems) != len(replica[it]) {
			t.Fatalf("wrong number of items for %v: %v != %v", it, len(idxItems), len(replica[it]))
		}
		for n := 0; n < len(idxItems); n++ {
			if idxItems[n].Epoch != replica[it][n].Epoch || idxItems[n].Offset != replica[it][n].Offset {
				t.Fatalf("wrong item for %v: %v != %v", it, idxItems[n], replica[it][n])
			}
		}
	}
	idx.Close()
	idx, err = OpenIndex("testdata", 8, 64, 8)
	if err != nil {
		t.Fatal(err)
	}
	for n := int64(0); n < 10000; n++ {
		idxItems := idx.Get(hashes[n], 0)
		it := testIndexToken(hashes[n][0:testIndexSize])
		if len(idxItems) != len(replica[it]) {
			t.Fatalf("wrong number of items for %v: %v != %v", it, len(idxItems), len(replica[it]))
		}
		for n := 0; n < len(idxItems); n++ {
			if idxItems[n].Epoch != replica[it][n].Epoch || idxItems[n].Offset != replica[it][n].Offset {
				t.Fatalf("wrong item for %v: %v != %v", it, idxItems[n], replica[it][n])
			}
		}
	}
}
