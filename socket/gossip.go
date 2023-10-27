package socket

import (
	"log/slog"
	"time"

	"github.com/freehandle/breeze/crypto"
)

const PingPongInterval = time.Second

type ChannelConnection struct {
	Conn    *SignedConnection
	Signal  chan []byte
	Release chan struct{}
	Iddle   bool
	Live    bool
}

func (c *ChannelConnection) Activate() {
	c.Iddle = false
}

func (c *ChannelConnection) Sleep() {
	c.Iddle = true
}

func (c *ChannelConnection) Send(msg []byte) {
	if c.Live {
		c.Conn.Send(msg)
	}
}

func NewChannelConnection(conn *SignedConnection) *ChannelConnection {
	channel := &ChannelConnection{
		Conn:    conn,
		Signal:  make(chan []byte),
		Release: make(chan struct{}),
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("ChannelConnection: recovered from panic", "error", r)
			}
		}()
		for {
			data, err := conn.Read()
			if err != nil {
				slog.Info("channel connection: could not read data", "error", err)
				channel.Live = false
				close(channel.Signal)
				return
			}
			if !channel.Iddle {
				// ignore empty messages of ping/pong messages
				if len(data) > 0 && (len(data) != 1 || data[0] != 255) {
					channel.Signal <- data
				}
			}
		}
	}()

	go func() {
		// ping/pong beat while iddle to attest connection is alve
		for {
			time.Sleep(PingPongInterval)
			if !channel.Live {
				return
			}
			if channel.Iddle {
				err := channel.Conn.Send([]byte{255})
				if err != nil {
					channel.Live = false
					channel.Conn.Shutdown()
					return
				}
			}
		}
	}()

	return channel
}

type GossipMessage struct {
	Signal []byte
	Token  crypto.Token
}

type Gossip struct {
	members []*ChannelConnection
	Signal  chan GossipMessage
	hashes  map[crypto.Hash]struct{}
}

func (g *Gossip) Release() {
	for _, conn := range g.members {
		conn.Release <- struct{}{}
	}
}

func (g *Gossip) ReleaseToken(token crypto.Token) {
	for _, conn := range g.members {
		if conn.Conn.Token.Equal(token) {
			conn.Release <- struct{}{}
		}
	}
}

func (g *Gossip) Broadcast(msg []byte) {
	for _, conn := range g.members {
		err := conn.Conn.Send(msg)
		if err != nil {
			slog.Info("gossip network: could not send message", "token", conn.Conn.Token, "error", err)
		}
	}
}

func (g *Gossip) BroadcastExcept(msg []byte, token crypto.Token) {
	for _, conn := range g.members {
		if !conn.Conn.Token.Equal(token) {
			err := conn.Conn.Send(msg)
			if err != nil {
				slog.Info("gossip network: could not send message", "token", conn.Conn.Token, "error", err)
			}
		}
	}
}

func NewGossip(connections []*ChannelConnection) *Gossip {
	gossip := &Gossip{
		members: connections,
		Signal:  make(chan GossipMessage),
		hashes:  make(map[crypto.Hash]struct{}),
	}
	for _, connection := range connections {
		go func(conn *ChannelConnection) {
			for {
				select {
				case <-conn.Release:
					return
				case msg := <-conn.Signal:
					hash := crypto.Hasher(msg)
					if _, ok := gossip.hashes[hash]; !ok {
						gossip.Signal <- GossipMessage{Signal: msg, Token: conn.Conn.Token}
					}
				}
			}
		}(connection)
	}
	return gossip
}
