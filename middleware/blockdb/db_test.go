package blockdb

import (
	"fmt"
	"testing"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

var testConfig = DBConfig{
	Path:           "",
	Indexed:        true,
	ItemsPerBucket: 8,
	BitsForBucket:  10,
	IndexSize:      8,
}

func TestBlockDatabase(t *testing.T) {
	blockchain, err := NewBlockchainHistory(testConfig)
	if err != nil {
		t.Error(err)
	}
	if blockchain == nil {
		t.Error("blockchain is nil")
	}
	for epoch := 1; epoch < 10; epoch++ {
		block := IndexedBlock{
			Epoch: uint64(epoch),
			Items: make([]IndexItem, 0),
		}

		data := []byte(fmt.Sprintf("header for epoch %d", epoch))
		util.PutUint32(10, &data)
		offsets := len(data)
		for n := 0; n < 10; n++ {
			action := []byte(fmt.Sprintf("epoch %d item %d", epoch, n))
			util.PutByteArray(action, &data)
			hash := crypto.Hasher(action)
			block.Items = append(block.Items, IndexItem{
				Hash:   hash,
				Offset: offsets,
			})
			offsets += len(action) + 2
		}
		block.Data = data
		err := blockchain.IncorporateBlock(&block)
		if err != nil {
			t.Error(err)
		}
	}
	for epoch := 1; epoch < 10; epoch++ {
		for n := 0; n < 10; n++ {
			action := fmt.Sprintf("epoch %d item %d", epoch, n)
			hash := crypto.Hasher([]byte(action))
			found := blockchain.Find(hash, uint64(epoch))
			if len(found) != 1 {
				t.Fatal("expected 1 found", len(found))
			}
			if string(found[0]) != action {
				t.Fatal(action, "<>", string(found[0]))
			}
		}
	}
}
