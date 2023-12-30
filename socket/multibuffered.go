package socket

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

// BufferedChannel channels data from a signed connection to byte array channel.
// Data is read from the connection and store in a buffer until it is read from
// the channel. The channel will block when not data is available. BufferedChannel
// offers two channels per connection. Data on transit over the main channel is
// prepended with a zero byte, data on the side channel prepended with a one byte.
// The structure also offers a Len() method to check the number of bufferred
// messages in each channel.
type BufferedMultiChannel struct {
	Conn    *SignedConnection
	Live    bool
	next    chan uint64
	release chan uint64
	close   chan struct{}
	side    chan []byte
	read    map[uint64]chan []byte
}

// Is returns true if the token of the connection is equal to the given token.
func (b *BufferedMultiChannel) Is(token crypto.Token) bool {
	return b.Conn.Token.Equal(token)
}

func (b *BufferedMultiChannel) Shutdown() {
	if !b.Live {
		return
	}
	b.close <- struct{}{}
}

// Read reads data from the main channel buffer. If the buffer is empty, it
// blocks until data is available.
func (b *BufferedMultiChannel) Read(epoch uint64) []byte {
	if epoch == 0 || !b.Live {
		return nil
	}
	b.next <- epoch
	reader, ok := b.read[epoch]
	if !ok {
		return nil
	}
	data, ok := <-reader
	if !ok {
		return nil
	}
	return data
}

// Read reads data from the side channel buffer. If the buffer is empty, it
// blocks until data is available.
func (b *BufferedMultiChannel) ReadSide() []byte {
	if !b.Live {
		return nil
	}
	b.next <- 0 // zero epoch used to retrieve side channel data
	data, ok := <-b.side
	if !ok {
		return nil
	}
	return data
}

// Send a message through the side channel. The message is prepended with eight
// zero bytes.
func (b *BufferedMultiChannel) SendSide(data []byte) {
	if !b.Live {
		return
	}
	bytes := util.Uint64ToBytes(0)
	b.Conn.Send(append(bytes, data...))
}

// Send messages through the main channel. The message is prepended with the
// given epoch.
func (b *BufferedMultiChannel) Send(epoch uint64, data []byte) {
	if epoch == 0 || !b.Live {
		return
	}
	bytes := util.Uint64ToBytes(epoch)
	b.Conn.Send(append(bytes, data...))
}

// Release the channel and buffer (if any) of the provided epoch.
func (b *BufferedMultiChannel) Release(epoch uint64) {
	if epoch == 0 || !b.Live {
		return
	}
	b.release <- epoch
}

// NewBufferredChannel returns a new BufferedChannel for the given connection.
func NewBufferredMultiChannel(conn *SignedConnection) *BufferedMultiChannel {
	buffered := &BufferedMultiChannel{
		Conn:    conn,
		next:    make(chan uint64),
		release: make(chan uint64),
		close:   make(chan struct{}),
		read:    make(map[uint64]chan []byte),
		side:    make(chan []byte),
		Live:    true,
	}

	queue := make(chan []byte, 2)

	go func() {
		buffers := make(map[uint64][][]byte)
		waiting := make(map[uint64]bool)
		sideBuffer := make([][]byte, 0)
		waitingSide := false
		for {
			select {
			case data := <-queue:
				if len(data) < 8 {
					continue
				}
				epoch, _ := util.ParseUint64(data, 0)
				if epoch == 0 {
					if waitingSide {
						buffered.side <- data[8:]
						waitingSide = false
					} else {
						sideBuffer = append(sideBuffer, data[8:])
					}
				} else if buffer, ok := buffers[epoch]; ok {
					reader, ok := buffered.read[epoch]
					if !ok {
						buffers[epoch] = append(buffer, data[8:])
					} else {
						if waiting[epoch] {
							reader <- data[8:]
							waiting[epoch] = false
						} else {
							buffers[epoch] = append(buffer, data[8:])
						}
					}
				} else {
					if waiting[epoch] {
						buffered.read[epoch] <- data[8:]
						waiting[epoch] = false
					} else {
						buffers[epoch] = [][]byte{data[8:]}
					}
				}
			case epoch := <-buffered.next:
				if epoch == 0 {
					if len(sideBuffer) == 0 {
						// mark as waiting if there is no message to send immediately
						waitingSide = true
					} else {
						// send the oldest message in the side buffer
						buffered.side <- sideBuffer[0]
						sideBuffer = sideBuffer[1:]
					}
				} else {
					// find the reader channel or create one if non-existent
					reader, ok := buffered.read[epoch]
					if !ok {
						reader = make(chan []byte)
						buffered.read[epoch] = reader
					}
					// send the oldest message in the buffer or mark as waiting
					if buffer, ok := buffers[epoch]; ok {
						if len(buffer) == 0 {
							waiting[epoch] = true
						} else {
							reader <- buffer[0]
							buffers[epoch] = buffer[1:]
						}
					} else {
						waiting[epoch] = true
					}
				}
			case <-buffered.close:
				buffered.Live = false
				for _, r := range buffered.read {
					close(r)
				}
				close(buffered.side)
				close(buffered.next)
				close(buffered.close)
				close(queue)
				buffered.Conn.Shutdown()
				return
			case epoch := <-buffered.release:
				if reader, ok := buffered.read[epoch]; ok {
					close(reader)
					delete(buffered.read, epoch)
					delete(buffers, epoch)
					delete(waiting, epoch)
				}
			}
		}
	}()

	go func() {
		for {
			data, err := conn.Read()
			if err != nil {
				buffered.Shutdown()
				return
			}
			queue <- data
		}
	}()
	return buffered
}
