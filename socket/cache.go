package socket

import (
	"errors"
	"log"
	"log/slog"

	"github.com/freehandle/breeze/crypto"
)

type CachedConnection struct {
	Live  bool
	conn  *SignedConnection
	ready bool
	send  chan []byte
	queue chan struct{}
}

func (c *CachedConnection) Token() crypto.Token {
	return c.conn.Token
}

func (c *CachedConnection) Send(data []byte) {
	if c.Live {
		c.send <- data
	}
}

func (c *CachedConnection) SendDirect(data []byte) error {
	if !c.Live {
		return errors.New("connection is dead")
	}
	if len(data) == 0 {
		return nil
	}
	if err := c.conn.Send(data); err != nil {
		c.Live = false
		c.conn.Shutdown()
		return err
	}
	return nil
}

func (c *CachedConnection) Ready() {
	c.ready = true
	if c.Live {
		c.queue <- struct{}{}
	}
}

func (c *CachedConnection) Close() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in Close", "recover", r)
		}
	}()
	if c.Live {
		c.Live = false
		c.send <- nil
	}
}

func NewCachedConnection(conn *SignedConnection) *CachedConnection {
	cached := &CachedConnection{
		Live:  true,
		conn:  conn,
		ready: false,
		send:  make(chan []byte),
		queue: make(chan struct{}, 3), // 3 is to incorporate Ready event
	}

	msgCache := make([][]byte, 0)

	// send loop

	go func() {
		defer func() {
			cached.Live = false
			conn.Shutdown()
			close(cached.send)
			close(cached.queue)
		}()
		for {
			select {
			case <-cached.queue:
				if N := len(msgCache); N > 0 {
					data := msgCache[0]
					msgCache = msgCache[1:]
					if err := cached.SendDirect(data); err != nil {
						log.Printf("error sending data: %v", err)
						return
					}
					if N > 1 {
						// this will never block because there will be one
						// buffer slot on the channel
						cached.queue <- struct{}{}
					}
				}
			case data := <-cached.send:
				if data == nil {
					return
				}
				msgCache = append(msgCache, data)
				if cached.ready && len(cached.queue) < cap(cached.queue) {
					cached.queue <- struct{}{}
				}
			}
		}
	}()
	return cached
}
