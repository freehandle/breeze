package socket

import (
	"testing"
	"time"

	"github.com/freehandle/breeze/crypto"
)

func TestTestingNetwork(t *testing.T) {
	TCPNetworkTest.AddNode("node1", 1, 100*time.Millisecond, 1e9)
	TCPNetworkTest.AddNode("node2", 1, 100*time.Millisecond, 1e9)
	_, pk1 := crypto.RandomAsymetricKey()
	_, pk2 := crypto.RandomAsymetricKey()
	go func() {
		lister, err := Listen("node1:7400")
		if err != nil {
			t.Error(err)
		}
		conn, err := lister.Accept()
		if err != nil {
			t.Error(err)
		}
		trusted, err := PromoteConnection(conn, pk1, AcceptAllConnections)
		if err != nil {
			t.Error(err)
		}
		trusted.Send([]byte("hello"))
	}()
	conn, err := Dial("node2", "node1:7400", pk2, pk1.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := conn.Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) != "hello" {
		t.Fatal("wrong message")
	}
}
