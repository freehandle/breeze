package socket

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/freehandle/breeze/crypto"
)

const CommitteeRetries = 1

var CommitteeRetryDelay = time.Second

type CommitteeMember struct {
	Address string
	Token   crypto.Token
}

type committeePool[T TokenComparer] struct {
	mu        *sync.Mutex
	connected []T
	remaining []CommitteeMember
	token     crypto.Token
}

type TokenComparer interface {
	Is(crypto.Token) bool
}

func addToPool[T TokenComparer](conn *SignedConnection, pool *committeePool[T], NewT func(conn *SignedConnection) T) (bool, int) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	isnew := true
	for _, r := range pool.connected {
		if r.Is(pool.token) {
			isnew = false
		}
	}
	if isnew {
		channel := NewT(conn)
		pool.connected = append(pool.connected, channel)
	}
	for n, r := range pool.remaining {
		if conn.Token.Equal(r.Token) {
			pool.remaining = append(pool.remaining[:n], pool.remaining[n+1:]...)
			break
		}
	}
	return isnew, len(pool.remaining)
}

func isMember[T TokenComparer](token crypto.Token, pool *committeePool[T]) bool {
	for _, r := range pool.connected {
		if r.Is(token) {
			return true
		}
	}
	return false
}

func newPool[T TokenComparer](peers []CommitteeMember, connected []T, token crypto.Token, NewT func(conn *SignedConnection) T) *committeePool[T] {
	pool := &committeePool[T]{
		mu:        &sync.Mutex{},
		connected: connected,
		remaining: make([]CommitteeMember, 0),
		token:     token,
	}
	for _, peer := range peers {
		exists := false
		if peer.Token.Equal(token) {
			continue
		}
		for _, conn := range connected {
			if conn.Is(peer.Token) {
				exists = true
				break
			}
		}
		if !exists {
			pool.remaining = append(pool.remaining, peer)
		}
	}
	return pool
}

func AssembleCommittee[T TokenComparer](peers []CommitteeMember, connected []T, NewT func(*SignedConnection) T, credentials crypto.PrivateKey, port int, hostname string) chan []T {
	done := make(chan []T, 2)
	pool := newPool(peers, connected, credentials.PublicKey(), NewT)

	listener, err := Listen(fmt.Sprintf(":%v", port))
	if err != nil {
		slog.Warn("BuilderCommittee: could not listen on port", "port", port, "error", err)
		done <- nil
		return done
	}

	for _, peer := range pool.remaining {
		go func(address string, token crypto.Token) {
			for n := 0; n < CommitteeRetries; n++ {
				time.Sleep(200 * time.Millisecond)
				conn, err := Dial(hostname, address, credentials, token)
				if err == nil {
					ok, remaining := addToPool(conn, pool, NewT)
					if !ok {
						conn.Shutdown()
					}
					if remaining == 0 {
						listener.Close()
					}
					return
				}
			}
			if !isMember(token, pool) {
				slog.Info("BuilderCommittee: could not connect to peer", "address", address, "error", err)
			}
		}(peer.Address, peer.Token)
	}

	go func() {
		tokens := make([]crypto.Token, 0)
		for _, member := range peers {
			tokens = append(tokens, member.Token)
		}
		validConnections := NewValidConnections(tokens)
		for {
			if conn, err := listener.Accept(); err == nil {
				trustedConn, err := PromoteConnection(conn, credentials, validConnections)
				if err == nil {
					_, remaining := addToPool(trustedConn, pool, NewT)
					if remaining == 0 {
						listener.Close()
						break
					}
				} else {
					conn.Close()
				}
			} else {
				break
			}
		}
		done <- pool.connected
	}()

	return done
}
