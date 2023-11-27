package socket

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

const (
	PingPongInterval = time.Second
	//ChannelConnMaxBuffer = 1000
)

type ChannelConnection struct {
	Conn    *SignedConnection
	Signal  map[uint64]chan []byte
	release chan uint64
	Iddle   bool
	Live    bool
}

func (c *ChannelConnection) Is(token crypto.Token) bool {
	return c.Conn.Token.Equal(token)
}

func (c *ChannelConnection) Activate() {
	c.Iddle = false
}

func (c *ChannelConnection) Sleep() {
	c.Iddle = true
}

func (c *ChannelConnection) Register(epoch uint64, signal chan []byte) {
	c.Signal[epoch] = signal
}

func (c *ChannelConnection) Release(epoch uint64) {
	c.release <- epoch
}

func (c *ChannelConnection) Send(msg []byte) {
	if c.Live {
		c.Conn.Send(msg)
	}
}

func (c *ChannelConnection) Read(epoch uint64) []byte {
	reader, ok := c.Signal[epoch]
	if !ok {
		return nil
	}
	return <-reader
}

func NewChannelConnection(conn *SignedConnection) *ChannelConnection {
	channel := &ChannelConnection{
		Conn:    conn,
		Signal:  make(map[uint64]chan []byte),
		release: make(chan uint64),
		Live:    true,
	}

	allsignals := make(chan []byte)

	go func() {
		for {
			select {
			case epoch := <-channel.release:
				// Release all signals
				if epoch == 0 {
					for _, signal := range channel.Signal {
						close(signal)
					}
					channel.Signal = make(map[uint64]chan []byte)
					channel.Iddle = true
				}
				// Release specific signal
				if signal, ok := channel.Signal[epoch]; ok {
					close(signal)
					delete(channel.Signal, epoch)
				}
			case data := <-allsignals:
				if len(data) >= 9 {
					epoch, _ := util.ParseUint64(data, 1)
					if signal, ok := channel.Signal[epoch]; ok {
						signal <- data
					}
				}
			}
		}
	}()

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
				for _, signal := range channel.Signal {
					close(signal)
				}
				return
			}
			if !channel.Iddle {
				if len(data) > 9 {
					allsignals <- data
				} else if len(data) == 1 && data[0] == 0 {
					channel.release <- 0
					channel.Iddle = true
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
	epoch   uint64
	members []*ChannelConnection
	Signal  chan GossipMessage
	hashes  map[crypto.Hash]struct{}
}

func (g *Gossip) Messages() chan GossipMessage {
	return g.Signal
}

func (g *Gossip) Release() {
	for _, conn := range g.members {
		conn.release <- g.epoch
	}
}

func (g *Gossip) ReleaseToken(token crypto.Token) {
	for _, conn := range g.members {
		if conn.Conn.Token.Equal(token) {
			conn.release <- g.epoch
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

func GroupGossip(epoch uint64, connections []*ChannelConnection) *Gossip {
	gossip := &Gossip{
		members: connections,
		Signal:  make(chan GossipMessage),
		hashes:  make(map[crypto.Hash]struct{}),
	}
	for _, connection := range connections {
		go func(conn *ChannelConnection) {
			signal := make(chan []byte)
			conn.Register(epoch, signal)
			for {
				msg, ok := <-signal
				if !ok {
					return
				}
				hash := crypto.Hasher(msg)
				if _, ok := gossip.hashes[hash]; !ok {
					gossip.Signal <- GossipMessage{Signal: msg, Token: conn.Conn.Token}
				}
			}
		}(connection)
	}
	return gossip
}

/*
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
*/

func AssembleChannelNetwork(peers []CommitteeMember, credentials crypto.PrivateKey, port int, existing []*ChannelConnection) []*ChannelConnection {
	for n, peer := range peers {
		peers[n] = CommitteeMember{
			Address: fmt.Sprintf("%v:%v", peer.Address, port),
			Token:   peer.Token,
		}
	}
	committee := AssembleCommittee[*ChannelConnection](peers, existing, NewChannelConnection, credentials, port)
	members := <-committee
	return members
}
