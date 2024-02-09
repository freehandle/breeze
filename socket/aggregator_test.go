package socket

import (
	"context"
	"fmt"
	"testing"

	"github.com/freehandle/breeze/crypto"
)

func senderTest(ctx context.Context, pk crypto.PrivateKey, port int, errResp chan error) {
	listner, err := Listen(fmt.Sprintf("localhost:%d", port))
	conn, err := listner.Accept()
	if err != nil {
		errResp <- err
		return
	}
	trusted, err := PromoteConnection(conn, pk, AcceptAllConnections)
	if err != nil {
		conn.Close()
		errResp <- err
		return
	}
	trusted.Send([]byte("hello world"))
	trusted.Send([]byte(fmt.Sprintf("from port %d", port)))
	<-ctx.Done()
	trusted.Shutdown()
}

func TestAggregator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var pks [5]crypto.PrivateKey
	errResp := make(chan error)
	for n := 0; n < len(pks); n++ {
		_, pks[n] = crypto.RandomAsymetricKey()
		go senderTest(ctx, pks[n], 7400+n, errResp)
	}
	conns := make([]*SignedConnection, 5)
	var err error
	for n := 0; n < len(pks); n++ {
		conns[n], err = Dial("localhost", fmt.Sprintf("localhost:%d", 7400+n), pks[n], pks[n].PublicKey())
		if err != nil {
			t.Fatal(err)
		}
	}
	agg := NewAgregator(ctx, "localhost", pks[0], conns...)
	if agg == nil {
		t.Fatal("aggregator is nil")
	}
	if len(agg.providers) != 5 {
		t.Fatal("wrong number of providers")
	}
	messages := map[string]int{
		"hello world":    5,
		"from port 7400": 1,
		"from port 7401": 1,
		"from port 7402": 1,
		"from port 7403": 1,
		"from port 7404": 1,
	}
	for {
		fmt.Println("messages left:", len(messages))
		msg, err := agg.Read()
		fmt.Println(string(msg))
		if err != nil {
			t.Fatal(err)
		}
		if count := messages[string(msg)]; count == 0 {
			t.Fatalf("unexpected message: %s", string(msg))
		} else if count == 1 {
			delete(messages, string(msg))
		} else {
			messages[string(msg)] = count - 1
		}
		if len(messages) == 0 {
			break
		}
	}
	cancel()

}
