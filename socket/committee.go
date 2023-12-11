package socket

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/freehandle/breeze/crypto"
)

const CommitteeRetries = 1 // number of retries to connect to a peer before giving up

var CommitteeRetryDelay = time.Second // should wait for this period before retrying

// CommitteeMember is a node in the committee. Address should be reachable and
// a signed connecition for the given token should be possible.
type CommitteeMember struct {
	Address string
	Token   crypto.Token
}

// committeePool keeps track of the nodes already connected and those that are
// still remaining.
type committeePool[T TokenComparer] struct {
	mu        *sync.Mutex
	connected []T
	remaining []CommitteeMember
	token     crypto.Token
}

// TokenComparer is an interface for comparing a token to a given token. The
// pool assemblage will use this to check if a given token is already connected.
type TokenComparer interface {
	Is(crypto.Token) bool
}

// addToPool adds a new signed connection to the connection of the pool.
// It returns true if the connection is new and the number of remaining
// connections to be established.
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

// isMember checks if a given token is already connected to the pool.
func isMember[T TokenComparer](token crypto.Token, pool *committeePool[T]) bool {
	for _, r := range pool.connected {
		if r.Is(token) {
			return true
		}
	}
	return false
}

// newPool creates a new committeePool object. if will populates the connected
// field with all existig connections declared in the peer froup and populate
// the remaining field with all the peers that are not connected.
// NewT is a function that creates a new T object from a signed connection.
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

// AssembleCommittee assembles a committee of nodes. It returns a channel for
// the slice of connections. The channel will be populated with all the
// connections that were possible to establish. The caller is responsible to
// attest if the pool is acceptable or not.
// peers is the list of peers expected in the committee. connected is the list
// of live connections. NewT is a function that creates a new T object from a
// signed connection. credentials is the private key of the node. port is the
// port to listen on for new connections (other nodes will try to assemble the
// pool at the same time). hostname is "localhost" or "" for internet connections
// anything else for testing.
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
