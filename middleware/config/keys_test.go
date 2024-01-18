package config

import (
	"context"
	"testing"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

func TestKeySync(t *testing.T) {
	socket.TCPNetworkTest.AddNode("node1", 1, 100*time.Millisecond, 1e9)
	socket.TCPNetworkTest.AddNode("node2", 1, 100*time.Millisecond, 1e9)
	tk0, pk0 := crypto.RandomAsymetricKey()
	tk1, pk1 := crypto.RandomAsymetricKey()
	tk2, pk2 := crypto.RandomAsymetricKey()
	tk3, pk3 := crypto.RandomAsymetricKey()

	tokens := []crypto.Token{tk1, tk2, tk3}
	vault := map[crypto.Token]crypto.PrivateKey{
		tk1: pk1,
		tk2: pk2,
		tk3: pk3,
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(200 * time.Millisecond)
		conn, err := socket.Dial("node2", "node1:1234", pk1, tk0)
		if err != nil {
			cancel()
			return
		}
		ok := DiffieHellmanExchangeClient(conn, vault)
		if !ok {
			cancel()
			return
		}
	}()

	keys := waitForRemoteKeysSyncWithTempKey(ctx, tokens, pk0, "node1", 1234)
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}
