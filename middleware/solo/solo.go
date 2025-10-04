package solo

import (
	"context"
	"sync"

	"fmt"
	"io"
	"net"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/social"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

type SimpleBlock struct {
	Epoch   uint64
	Actions [][]byte
}

func OpenAndListen(ctx context.Context, address string, token crypto.Token, credentials crypto.PrivateKey, epoch uint64, r io.ReadCloser) chan *SimpleBlock {
	out := make(chan *SimpleBlock, 1)
	go func() {
		if err := ReadChain(r, out); err != nil {
			close(out)
			return
		}
		if err := ListenBlocks(ctx, address, token, credentials, epoch, out); err != nil {
			close(out)
			return
		}
	}()
	return out
}

func ListenBlocks(ctx context.Context, address string, token crypto.Token, credentials crypto.PrivateKey, epoch uint64, out chan *SimpleBlock) error {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		close(out)
		return err
	}
	signed, err := socket.PromoteConnection(conn, credentials, socket.AcceptAllConnections)
	if err != nil {
		close(out)
		conn.Close()
		return err
	}
	data := util.Uint64ToBytes(epoch)
	err = signed.Send(data)
	if err != nil {
		signed.Shutdown()
		close(out)
		return err
	}
	go func() {
		data, err := signed.Read()
		if len(data) != 8 || err != nil {
			close(out)
			signed.Shutdown()
			return
		}
		startEpoch, _ := util.ParseUint64(data, 0)
		if startEpoch != epoch+1 {
			close(out)
			signed.Shutdown()
			return
		}
		block := &SimpleBlock{
			Epoch:   startEpoch - 1,
			Actions: make([][]byte, 0),
		}
		for {
			data, err := signed.Read()
			if err != nil {
				close(out)
				signed.Shutdown()
				return
			}
			if len(data) == 8 {
				epoch, _ := util.ParseUint64(data, 0)
				if epoch != block.Epoch+1 {
					close(out)
					signed.Shutdown()
					return
				}
				out <- block
				block = &SimpleBlock{
					Epoch:   epoch,
					Actions: make([][]byte, 0),
				}
				continue
			}
		}
	}()
	return nil
}

type buffer struct {
	data []byte
	r    io.ReadCloser
	eof  bool
}

func (b *buffer) NextN(n int) ([]byte, error) {
	if n > len(b.data) {
		buffer := make([]byte, n)
		_, err := b.r.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if err == io.EOF {
			b.eof = true
			b.r.Close()
		}
		b.data = append(b.data, buffer...)
	}
	if n > len(b.data) {
		resto := b.data
		b.data = b.data[:0]
		return resto, nil
	}
	resto := b.data[:n]
	b.data = b.data[n:]
	return resto, nil
}

func NewBuffer(r io.ReadCloser) (*buffer, error) {
	b := &buffer{
		data: make([]byte, 1<<20),
		r:    r,
	}
	_, err := r.Read(b.data)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if err == io.EOF {
		b.eof = true
		r.Close()
	}
	return b, nil
}

func ReadChain(source io.ReadCloser, out chan *SimpleBlock) error {
	defer close(out)
	buffer, err := NewBuffer(source)
	if err != nil {
		return err
	}
	var epoch uint64
	var n int
	var count uint32
	block := &SimpleBlock{
		Epoch:   0,
		Actions: make([][]byte, 0),
	}
	for {
		data, err := buffer.NextN(8 + 4)
		if err != nil {
			return err
		}
		if len(data) < 12 {
			return nil
		}
		epoch, n = util.ParseUint64(data, 0)
		count, _ = util.ParseUint32(data, n)
		if block.Epoch != 0 {
			out <- block
		}
		block = &SimpleBlock{
			Epoch:   epoch,
			Actions: make([][]byte, 0, count),
		}
		for i := 0; i < int(count); i++ {
			data, err := buffer.NextN(4)
			if err != nil {
				return err
			}
			if len(data) < 4 {
				return nil
			}
			length := int(data[0]) | int(data[1])<<8 | int(data[2])<<16 | int(data[3])<<24
			if length == 0 {
				continue
			}
			data, err = buffer.NextN(length)
			if err != nil {
				return err
			}
			if len(data) < length {
				return nil
			}
			block.Actions = append(block.Actions, data)
		}
	}
}

type SoloChain[M social.Merger[M], B social.Blocker[M]] struct {
	Interval    time.Duration
	ListenPort  int
	GatewayPort int
	IO          io.WriteCloser
	State       social.Stateful[M, B]
	Epoch       uint64
	Recent      [][][]byte
	Keep        int
}

// Very simple chain implementation for local storage
func (solo *SoloChain[M, B]) PeristActions(actions [][]byte) error {
	bytes := make([]byte, 0)
	util.PutUint64(solo.Epoch, &bytes)
	util.PutUint32(uint32(len(actions)), &bytes)
	for _, action := range actions {
		util.PutLargeByteArray(action, &bytes)
	}
	_, err := solo.IO.Write(bytes)
	if err != nil {
		return err
	}
	if len(solo.Recent) < solo.Keep {
		solo.Recent = append(solo.Recent, actions)
	} else {
		solo.Recent = append(solo.Recent[1:], actions)
	}
	solo.Epoch += 1
	return nil
}

type ConnSyncRequest struct {
	Conn        *socket.SignedConnection
	SyncedEpoch uint64
}

func NewListeners() *Listeners {
	return &Listeners{
		mu:          sync.Mutex{},
		Connections: make([]*socket.SignedConnection, 0),
		Standby:     make(map[*socket.SignedConnection][][]byte),
		Released:    make(map[*socket.SignedConnection]struct{}),
	}
}

type Listeners struct {
	mu          sync.Mutex
	Connections []*socket.SignedConnection
	Standby     map[*socket.SignedConnection][][]byte
	Released    map[*socket.SignedConnection]struct{}
}

func (l *Listeners) Shutdown() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, conn := range l.Connections {
		conn.Shutdown()
	}
	for conn := range l.Standby {
		conn.Shutdown()
	}
}

func (l *Listeners) Add(conn *socket.SignedConnection, job [][][]byte, epoch uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Standby[conn] = make([][]byte, 0)
	go func() {
		for _, EpochData := range job {
			data := util.Uint64ToBytes(epoch)
			epoch += 1
			conn.Send(data)
			for _, action := range EpochData {
				conn.Send(action)
			}
		}
		l.Release(conn)
	}()
}

func (l *Listeners) Release(conn *socket.SignedConnection) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Released[conn] = struct{}{}
	go func() {
		for {
			l.mu.Lock()
			buffered := l.Standby[conn]
			if len(buffered) == 0 {
				l.mu.Unlock()
				return
			}
			data := buffered[0]
			l.Standby[conn] = buffered[1:]
			l.mu.Unlock()
			err := conn.Send(data)
			if err != nil {
				conn.Shutdown()
				l.mu.Lock()
				delete(l.Standby, conn)
				delete(l.Released, conn)
				l.mu.Unlock()
				return
			}
		}
	}()
}

func (l *Listeners) Broadcast(data []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	nConn := 0
	for {
		err := l.Connections[nConn].Send(data)
		if err != nil {
			l.Connections = append(l.Connections[:nConn], l.Connections[nConn+1:]...)
		} else {
			nConn += 1
		}
		if nConn >= len(l.Connections) {
			break
		}
	}
	incorporated := make([]*socket.SignedConnection, 0)
	for conn, buffered := range l.Standby {
		if _, ok := l.Released[conn]; ok && len(buffered) == 0 {
			l.Connections = append(l.Connections, conn)
			incorporated = append(incorporated, conn)
			delete(l.Released, conn)
		}
		conn.Send(data)
	}
	for _, conn := range incorporated {
		delete(l.Standby, conn)
	}
}

func (solo *SoloChain[M, B]) Start(ctx context.Context, credentials crypto.PrivateKey) chan error {
	ticker := time.NewTicker(solo.Interval)
	finalize := make(chan error, 2)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", solo.ListenPort))
	if err != nil {
		finalize <- err
		return finalize
	}
	gateway, err := net.Listen("tcp", fmt.Sprintf(":%d", solo.GatewayPort))
	if err != nil {
		finalize <- err
		listener.Close()
		return finalize
	}
	ctxCancel, cancel := context.WithCancel(ctx)
	newListener := make(chan ConnSyncRequest)
	receiver := make(chan []byte, 1)

	listeners := NewListeners()
	connected := make([]*socket.SignedConnection, 0)

	go func() {
		for {
			conn, err := gateway.Accept()
			if err != nil {
				listener.Close()
				return
			}
			signed, err := socket.PromoteConnection(conn, credentials, socket.AcceptAllConnections)
			if err != nil {
				continue
			}
			go func() {
				for {
					data, err := signed.Read()
					if err != nil {
						return
					}
					receiver <- data
				}
			}()
		}
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				cancel()
				finalize <- err
				return
			}
			signed, err := socket.PromoteConnection(conn, credentials, socket.AcceptAllConnections)
			if err != nil {
				continue
			}
			go func() {
				data, err := signed.Read()
				if len(data) != 8 || err != nil {
					signed.Shutdown()
					return
				}
				epoch, _ := util.ParseUint64(data, 0)
				newListener <- ConnSyncRequest{Conn: signed, SyncedEpoch: epoch}
			}()
		}
	}()
	go func() {
		validator := solo.State.Validator()
		actions := make([][]byte, 0)
		for {
			select {
			case <-ticker.C:
				solo.Epoch += 1
				mutations := validator.Mutations()
				solo.State.Incorporate(mutations)
				solo.PeristActions(actions)
				actions = actions[:0]
				epoch := util.Uint64ToBytes(solo.Epoch)
				listeners.Broadcast(epoch)
			case data := <-receiver:
				if len(data) == 0 || !solo.State.Validator().Validate(data) {
					continue
				}
				actions = append(actions, data)
				listeners.Broadcast(data)
			case <-ctxCancel.Done():
				for _, c := range connected {
					c.Shutdown()
				}
				solo.IO.Close()
				solo.State.Shutdown()
				listeners.Shutdown()
				ticker.Stop()
				finalize <- nil
			case listener := <-newListener:
				if listener.SyncedEpoch > solo.Epoch {
					listener.Conn.Shutdown()
					continue
				}
				count := int(solo.Epoch - listener.SyncedEpoch) // how many epoch to send
				job := append(solo.Recent[len(solo.Recent)-count+1:], actions)
				listeners.Add(listener.Conn, job, listener.SyncedEpoch+1)
			}
		}
	}()
	return finalize
}
