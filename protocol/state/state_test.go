package state

import (
	"testing"

	"github.com/freehandle/breeze/crypto"
)

func TestWalletClone(t *testing.T) {

	wallet := NewMemoryWalletStore("wallet", 6)
	balances := make(map[crypto.Token]uint64)

	for i := 0; i < 15000; i++ {
		token, _ := crypto.RandomAsymetricKey()
		if balance, ok := balances[token]; ok {
			balances[token] = balance + 1000
		} else {
			balances[token] = 1000
		}
		wallet.Credit(token, 1000)
	}
	wallet.Bytes()
	wallet2 := NewMemoryWalletStoreFromBytes("wallet", wallet.Bytes())

	for token, balance := range balances {
		if _, cloned := wallet2.Balance(token); cloned != balance {
			t.Errorf("%v != %v", balance, cloned)
		}
	}
}
