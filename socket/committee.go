package socket

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/freehandle/breeze/crypto"
)

const CommitteeRetries = 3

var CommitteeRetryDelay = time.Second

type CommitteeMember struct {
	Address string
	Token   crypto.Token
}

func BuildCommittee(peers []CommitteeMember, connected []*ChannelConnection, credentials crypto.PrivateKey, port int) chan []*ChannelConnection {
	done := make(chan []*ChannelConnection, 2)
	mu := sync.Mutex{}

	existing := make([]*ChannelConnection, 0)
	remaining := make([]CommitteeMember, 0)
	for _, peer := range peers {
		exists := false
		if peer.Token.Equal(credentials.PublicKey()) {
			continue
		}
		for _, conn := range connected {
			if conn.Conn.Token.Equal(peer.Token) {
				existing = append(existing, conn)
				exists = true
				break
			}
		}
		if !exists {
			remaining = append(remaining, peer)
		}
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%v", port))
	if err != nil {
		slog.Warn("BuilderCommittee: could not listen on port", "porT", port, "error", err)
		done <- nil
		return done
	}

	for _, peer := range peers {
		go func(address string, token crypto.Token) {
			for n := 0; n < CommitteeRetries; n++ {
				conn, err := Dial(peer.Address, credentials, peer.Token)
				if err == nil {
					channel := NewChannelConnection(conn)
					last := false
					mu.Lock()
					for n, r := range remaining {
						if r.Token.Equal(token) {
							remaining = append(remaining[:n], remaining[n+1:]...)
							break
						}
					}
					if len(remaining) == 0 {
						last = true
					}
					connected = append(connected, channel)
					mu.Unlock()
					if last {
						listener.Close()
					}
				}
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
				if err != nil {
					mu.Lock()
					last := false
					for n, r := range remaining {
						if r.Token.Equal(trustedConn.Token) {
							remaining = append(remaining[:n], remaining[n+1:]...)
							break
						}
					}
					if len(remaining) == 0 {
						last = true
					}
					mu.Unlock()
					if last {
						listener.Close()
						return
					}
				} else {
					conn.Close()
				}
			}
		}
	}()

	return done
}
