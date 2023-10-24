package socket

import (
	"github.com/freehandle/breeze/crypto"
)

type ConnectionPool map[crypto.Token]*CachedConnection

func (p ConnectionPool) Broadcast(data []byte) {
	for _, conn := range p {
		conn.Send(data)
	}
}

func (p ConnectionPool) Add(c *CachedConnection) {
	p[c.conn.Token] = c
}

func (p ConnectionPool) Drop(token crypto.Token) {
	if conn, ok := p[token]; ok {
		if conn != nil {
			conn.Live = false
			conn.Close()
		}
		delete(p, token)
	}
}

func (p ConnectionPool) DropDead() {
	for token, conn := range p {
		if !conn.Live {
			p.Drop(token)
		}
	}
}
