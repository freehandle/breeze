package bft

import (
	"sync"
	"testing"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type TestGossipNetwork struct {
	inbox map[crypto.Token]chan socket.GossipMessage
}

type TestGossipConnection struct {
	network *TestGossipNetwork
	token   crypto.Token
}

func (t *TestGossipConnection) Broadcast(msg []byte) {
	for token, pipe := range t.network.inbox {
		if !token.Equal(t.token) {
			pipe <- socket.GossipMessage{Signal: msg, Token: t.token}
		}
	}
}

func (t *TestGossipConnection) BroadcastExcept(msg []byte, except crypto.Token) {
	for token, pipe := range t.network.inbox {
		if !token.Equal(t.token) && !except.Equal(t.token) {
			pipe <- socket.GossipMessage{Signal: msg, Token: t.token}
		}
	}
}

func (t *TestGossipConnection) ReleaseToken(token crypto.Token) {
}

func (t *TestGossipConnection) Messages() chan socket.GossipMessage {
	hashes := make(map[crypto.Hash]struct{})
	processed := make(chan socket.GossipMessage)
	go func() {
		for {
			msg, ok := <-t.network.inbox[t.token]
			if !ok || len(msg.Signal) == 0 {
				close(processed)
				return
			}
			hash := crypto.Hasher(msg.Signal)
			if _, ok := hashes[hash]; !ok {
				//fmt.Printf("%v %v\n", msg.Signal[0], t.token)
				hashes[hash] = struct{}{}
				processed <- msg
			}
			//
		}
	}()
	return processed
}

// for a valid return value.
func TestPooling(t *testing.T) {
	members := make(map[crypto.Token]PoolingMembers)
	order := make([]crypto.Token, 0)
	network := &TestGossipNetwork{
		inbox: make(map[crypto.Token]chan socket.GossipMessage),
	}
	credentials := make([]crypto.PrivateKey, 12)
	for n := 0; n < 12; n++ {
		tk, pk := crypto.RandomAsymetricKey()
		credentials[n] = pk
		members[tk] = PoolingMembers{Weight: 1}
		order = append(order, tk)
		network.inbox[tk] = make(chan socket.GossipMessage, 12000)
	}
	hash := crypto.Hasher([]byte("test hash"))
	var wg sync.WaitGroup
	wg.Add(12)
	for n := 0; n < 12; n++ {
		go func(n int, credentials crypto.PrivateKey) {
			p := PoolingCommittee{
				Epoch:   0,
				Members: members,
				Order:   order,
				Gossip:  &TestGossipConnection{network: network, token: credentials.PublicKey()},
			}
			pools := LaunchPooling(p, credentials)
			time.Sleep(10 * time.Duration(n) * time.Millisecond)
			pools.SealBlock(hash)
			consensus := <-pools.Finalize
			wg.Done()
			if !consensus.Value.Equal(hash) {
				t.Error("consensus hash mismatch")
			}
		}(n, credentials[n])
	}
	wg.Wait()
}
