package socket

import (
	"github.com/freehandle/breeze/crypto"
)

// ConnectionPool is a map of cached connections to other nodes in the peer group.
type ConnectionPool map[crypto.Token]*CachedConnection

// Broadcast sends data to all nodes in the connection pool.
func (p ConnectionPool) Broadcast(data []byte) {
	for _, conn := range p {
		conn.Send(data)
	}
}

// Add adds a new cached connection to the connection pool.
func (p ConnectionPool) Add(c *CachedConnection) {
	p[c.conn.Token] = c
}

// DropAll closes all connections in the connection pool.
func (p ConnectionPool) DropAll() {
	for _, conn := range p {
		conn.Close()
	}
}

// Drop closes a connection in the connection pool.
func (p ConnectionPool) Drop(token crypto.Token) {
	if conn, ok := p[token]; ok {
		if conn != nil {
			conn.Live = false
			conn.Close()
		}
		delete(p, token)
	}
}

// DropDead closes all dead connections in the connection pool.
func (p ConnectionPool) DropDead() {
	for token, conn := range p {
		if !conn.Live {
			p.Drop(token)
		}
	}
}
