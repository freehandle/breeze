package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/socket"
)

func testAction() []byte {
	tk, pk := crypto.RandomAsymetricKey()
	deposit := actions.Deposit{
		TimeStamp: 1,
		Token:     tk,
		Value:     10,
		Fee:       1,
	}
	deposit.Sign(pk)
	return deposit.Serialize()
}

func TestActionVault(t *testing.T) {
	conn1, conn2 := socket.CreateConnectionPair("node", 7400)
	fmt.Println("ok")
	propose := make(chan *Propose)
	v := NewActionVault(context.Background(), 1, propose)
	if v == nil {
		t.Fatal("NewActionVault returned nil")
	}
	proposal := &Propose{
		data: testAction(),
		conn: conn1,
	}
	propose <- proposal
	v.NextEpoch()
	time.Sleep(10 * time.Millisecond)
	if v.clock != 2 {
		t.Fatal("NextEpoch did not increment epoch", v.clock)
	}
	if len(v.pending)+1 != 1 {
		t.Fatal("did not incorporate propose")
	}
	data := <-v.Pop
	if len(data) != len(proposal.data) {
		t.Fatal("did not pop propose", data, proposal.data)
	}
	resp, _ := conn2.Read()
	if len(resp) != 33 || resp[0] != messages.MsgActionForward {
		t.Fatal("did not forward", resp)
	}
	v.seal <- SealOnBlock{
		Epoch:     2,
		BlockHash: crypto.Hasher(data),
		Action:    data,
	}
	resp, _ = conn2.Read()
	_, epochSeal, _ := messages.ParseSealedAction(resp)
	if epochSeal != 2 {
		t.Fatal("did not seal", resp)
	}

}
