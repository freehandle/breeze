package socket

import (
	"log/slog"

	"github.com/freehandle/breeze/crypto"
)

// BufferedChannel buffers read data from a connection
type BufferedChannel struct {
	Conn      *SignedConnection
	Live      bool
	next      chan struct{}
	nextside  chan struct{}
	count     chan chan int
	countside chan chan int
	read      chan []byte
	readSide  chan []byte
}

func (b *BufferedChannel) Is(token crypto.Token) bool {
	return b.Conn.Token.Equal(token)
}

func (b *BufferedChannel) Read() []byte {
	b.next <- struct{}{}
	data, ok := <-b.read
	if !ok {
		close(b.next)
	}
	return data
}

func (b *BufferedChannel) ReadSide() []byte {
	b.nextside <- struct{}{}
	data, ok := <-b.readSide
	if !ok {
		close(b.nextside)
	}
	return data
}

func (b *BufferedChannel) Len() int {
	c := make(chan int)
	b.count <- c
	return <-c
}

func (b *BufferedChannel) Send(data []byte) {
	b.Conn.Send(append([]byte{0}, data...))
}

func (b *BufferedChannel) SendSide(data []byte) {
	b.Conn.Send(append([]byte{1}, data...))
}

func NewBufferredChannel(conn *SignedConnection) *BufferedChannel {
	buffered := &BufferedChannel{
		Conn:      conn,
		next:      make(chan struct{}),
		count:     make(chan chan int),
		read:      make(chan []byte),
		nextside:  make(chan struct{}),
		countside: make(chan chan int),
		readSide:  make(chan []byte),
	}

	queue := make(chan []byte, 2)

	go func() {
		buffer := make([][]byte, 0)
		bufferside := make([][]byte, 0)
		waiting := false
		waitingside := false
		for {
			select {
			case data := <-queue:
				if len(data) == 0 {
					close(buffered.read)
					close(queue)
					return
				}
				if data[0] == 0 {
					if waiting {
						buffered.read <- data
						waiting = false
					} else {
						buffer = append(buffer, data)
					}
				} else {
					if waitingside {
						buffered.readSide <- data
						waitingside = false
					} else {
						buffer = append(bufferside, data)
					}
				}
			case <-buffered.next:
				if len(buffer) == 0 {
					waiting = true
				} else {
					buffered.read <- buffer[0]
					buffer = buffer[1:]
				}
			case <-buffered.nextside:
				if len(bufferside) == 0 {
					waitingside = true
				} else {
					buffered.readSide <- bufferside[0]
					bufferside = bufferside[1:]
				}
			case count := <-buffered.count:
				count <- len(buffer)
			case count := <-buffered.countside:
				count <- len(bufferside)
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
