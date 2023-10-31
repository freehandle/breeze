package socket

import (
	"log/slog"

	"github.com/freehandle/breeze/crypto"
)

// BufferedChannel buffers read data from a connection
type BufferedChannel struct {
	Conn  *SignedConnection
	Live  bool
	next  chan struct{}
	count chan chan int
	send  chan []byte
}

func (b *BufferedChannel) Is(token crypto.Token) bool {
	return b.Conn.Token.Equal(token)
}

func (b *BufferedChannel) Read() []byte {
	b.next <- struct{}{}
	data, ok := <-b.send
	if !ok {
		close(b.next)
	}
	return data
}

func (b *BufferedChannel) Len() int {
	c := make(chan int)
	b.count <- c
	return <-c
}

func NewBufferredChannel(conn *SignedConnection) *BufferedChannel {
	buffered := &BufferedChannel{
		Conn:  conn,
		next:  make(chan struct{}),
		count: make(chan chan int),
		send:  make(chan []byte),
	}

	queue := make(chan []byte, 2)

	go func() {
		buffer := make([][]byte, 0)
		waiting := false
		for {
			select {
			case data := <-queue:
				if len(data) == 0 {
					close(buffered.send)
					close(queue)
					return
				}
				if waiting {
					buffered.send <- data
					waiting = false
				} else {
					buffer = append(buffer, data)
				}
			case <-buffered.next:
				if len(buffer) == 0 {
					waiting = true
				} else {
					buffered.send <- buffer[0]
					buffer = buffer[1:]
				}
			case count := <-buffered.count:
				count <- len(buffer)
			}

		}
	}()

	go func() {
		for {
			data, err := conn.Read()
			if err != nil {
				slog.Info("buffered channel: could not read data", "error", err)
				buffered.Live = false
				queue <- nil

			}
			queue <- data
		}
	}()
	return buffered
}

type BroadcastPool struct {
	members []*BufferedChannel
	leader  int
}

func (b *BroadcastPool) NewLeader(token crypto.Token) *BufferedChannel {
	for n, member := range b.members {
		if member.Is(token) {
			b.leader = n
			return member
		}
	}
	return nil
}

func (b *BroadcastPool) CountShift(shift int) int {
	node := (b.leader + shift) % len(b.members)
	return b.members[node].Len()
}

func (b *BroadcastPool) Send(data []byte) {
	for _, member := range b.members {
		member.Conn.Send(data)
	}
}

func AssembleBroadcastPool(peers []CommitteeMember, credentials crypto.PrivateKey, port int) *BroadcastPool {
	committee := AssembleCommittee[*BufferedChannel](peers, make([]*BufferedChannel, 0), NewBufferredChannel, credentials, port)
	members := <-committee
	return &BroadcastPool{
		members: members,
		leader:  0,
	}
}
