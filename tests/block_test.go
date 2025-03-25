package tests

import (
	"testing"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
)

func TestBlockchain(t *testing.T) {
	token, pk := crypto.RandomAsymetricKey()
	token2, _ := crypto.RandomAsymetricKey()
	hashToken := crypto.HashToken(token)
	testChain := chain.BlockchainFromGenesisState(pk, "", hashToken, time.Second, 15)
	block, err := testChain.BlockBuilder(1)
	if err != nil {
		t.Error(err)
	}
	if block == nil {
		t.Error("Block is nil")
	} else {
		if block.Header.Epoch != 1 {
			t.Error("First Block epoch is not 1")
		}
		if block.Header.CheckpointHash != hashToken {
			t.Error("First Block checkpoint hash is not genesis hash")
		}
		if block.Header.CheckPoint != 0 {
			t.Error("First Block checkpoint is not 0")
		}
		if block.Validator == nil {
			t.Error("First Block validator is nil")
		}
	}
	t1 := &actions.Transfer{
		TimeStamp: 1,
		From:      token,
		To:        []crypto.TokenValue{{Token: token2, Value: 10}},
		Reason:    "Whatever",
		Fee:       1,
	}
	t1.Sign(pk)
	ok := block.Validate(t1.Serialize())
	if !ok {
		t.Error("Failed to validate transfer")
	}
	sealed := block.Seal(pk)
	testChain.AddSealedBlock(sealed)
	if !testChain.CommitBlock(1) {
		t.Error("Failed to commit block")
	}
	if testChain.LastCommitEpoch != 1 {
		t.Error("Failed to update commit epoch")
	}
}
