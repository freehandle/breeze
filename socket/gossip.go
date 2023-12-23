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

// ChannelConnection is a wrapper around a SignedConnection that separates
// messages by epoch and routes them to dedicated byte array channels. The
// epoch is store between byte 1 and byte 8 of the message. If there is an
// open channel for that epoch, the message is sent to the channel. Otherwise
// it is discarded. The connection can be sent into iddle mode, in which case
// all messages received are simply ignored.
type ChannelConnection struct {
	Conn    *SignedConnection
	Signal  map[uint64]chan []byte
	release chan uint64
	Iddle   bool
	Live    bool
}

// Is returns true if the token of the connection is equal to the given token.
func (c *ChannelConnection) Is(token crypto.Token) bool {
	return c.Conn.Token.Equal(token)
}

// Activate sets the connection to active mode, in which case messages are
// routed to the corresponding channels.
func (c *ChannelConnection) Activate() {
	c.Iddle = false
}

// Sleep sets the connection to iddle mode, in which case messages are
// discarded.
func (c *ChannelConnection) Sleep() {
	c.Iddle = true
}

// Register registers a new channel for a given epoch.
func (c *ChannelConnection) Register(epoch uint64, signal chan []byte) {
	c.Signal[epoch] = signal
}

// Release releases a channel for a given epoch. If epoch = 0, it releases all
// channels and sets the connection to iddle. Released channels are closed
// and removed from the connection.
func (c *ChannelConnection) Release(epoch uint64) {
	c.release <- epoch
}

// Send sends a message to the remote node if the connection is live.
func (c *ChannelConnection) Send(msg []byte) {
	if c.Live {
		c.Conn.Send(msg)
	}
}

// Read reads a message from the channel corresponding to the given epoch. If
// there is no channel for that epoch, it returns nil. Otherwise it will block
// until a message is received.
func (c *ChannelConnection) Read(epoch uint64) []byte {
	reader, ok := c.Signal[epoch]
	if !ok {
		return nil
	}
	return <-reader
}

// NewChannelConnection returns a new ChannelConnection for the given signed
// connection.
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

// GossipMessage is a message received from the gossip network together with
// its author.
type GossipMessage struct {
	Signal []byte
	Token  crypto.Token
}

// Gossip is a group of connections where every node broadcasts to every other
// node new messages received. It is used as a communication primitive of the
// consensus committee. It should only be used for lightweight messages.
type Gossip struct {
	epoch   uint64
	members []*ChannelConnection
	Signal  chan GossipMessage
	hashes  map[crypto.Hash]struct{} // keep track of hashes to avoid duplicates
}

// Messages returns the GossipMessage channel og the network.
func (g *Gossip) Messages() chan GossipMessage {
	return g.Signal
}

// Release releases all channel connections of associated epoch in the gossip
// network.
func (g *Gossip) Release() {
	for _, conn := range g.members {
		conn.release <- g.epoch
	}
}

// Release a single channel connection of associated epoch in the gossip
// network.
func (g *Gossip) ReleaseToken(token crypto.Token) {
	for _, conn := range g.members {
		if conn.Conn.Token.Equal(token) {
			conn.release <- g.epoch
		}
	}
}

// Broadcast sends a message to all nodes in the gossip network.
func (g *Gossip) Broadcast(msg []byte) {
	for _, conn := range g.members {
		err := conn.Conn.Send(msg)
		if err != nil {
			slog.Info("gossip network: could not send message", "token", conn.Conn.Token, "error", err)
		}
	}
}

// BroadcastExcept sends a message to all nodes in the gossip network except
// the one with the given token.
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

// GroupGossip creates a new gossip network for a given epoch from a slice of
// ChannelConnections.
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

// AssembleChannelNetwork assembles a committee of ChannelConnections. It returns
// a channel for the slice of connections. The channel will be populated with all
// the connections that were possible to establish.
func AssembleChannelNetwork(peers []CommitteeMember, credentials crypto.PrivateKey, port int, hostname string, existing []*ChannelConnection) []*ChannelConnection {
	peersMembers := make([]CommitteeMember, len(peers))
	for n, peer := range peers {
		peersMembers[n] = CommitteeMember{
			Address: fmt.Sprintf("%v:%v", peer.Address, port),
			Token:   peer.Token,
		}
	}
	committee := AssembleCommittee[*ChannelConnection](peersMembers, existing, NewChannelConnection, credentials, port, hostname)
	members := <-committee
	return members
}
