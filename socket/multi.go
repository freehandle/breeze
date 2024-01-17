package socket

import (
	"context"
	"fmt"
	"sync"

	"github.com/freehandle/breeze/crypto"
)

func ConnectToAll(ctx context.Context, peers []TokenAddr, connected []*SignedConnection, credentials crypto.PrivateKey, port int, hostname string) chan []*SignedConnection {
	finished := make(chan []*SignedConnection, 2)
	live := make([]*SignedConnection, 0)
	remaining := make([]TokenAddr, 0)
	for _, peer := range peers {
		isNew := true
		for _, conn := range connected {
			if peer.Token.Equal(conn.Token) && peer.Addr == conn.Address {
				live = append(live, conn)
				isNew = false
			}
		}
		if isNew {
			remaining = append(remaining, TokenAddr{Token: peer.Token, Addr: fmt.Sprintf("%s:%d", peer.Addr, port)})
		}
	}
	if len(remaining) == 0 {
		finished <- live
		return finished
	}

	mu := sync.Mutex{}
	count := 0

	for _, peer := range remaining {
		go func(tk TokenAddr) {
			for n := 0; n < CommitteeRetries; n++ {
				conn, err := Dial(hostname, tk.Addr, credentials, tk.Token)
				if err == nil {
					mu.Lock()
					live = append(live, conn)
					count += 1
					if count == len(remaining) {
						mu.Unlock()
						finished <- live
						close(finished)
						return
					}
					mu.Unlock()
					return
				}
			}
			mu.Lock()
			count += 1
			if count == len(remaining) {
				mu.Unlock()
				finished <- live
				close(finished)
				return
			}
			mu.Unlock()
		}(peer)
	}
	return finished
}
