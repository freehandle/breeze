package socket

import (
	"errors"
	"fmt"
	"net"
	"time"
)

type dialer struct {
	latency  time.Duration
	response chan net.Conn
}

type fakePort struct {
	Port   string
	Node   *testHost
	dial   chan dialer
	accept chan net.Conn
	close  chan struct{}
	live   bool
}

func (f *fakePort) Close() error {
	if !f.live {
		return errors.New("port already closed")
	}
	f.close <- struct{}{}
	return nil
}

func (f *fakePort) Connect(latency time.Duration) (net.Conn, error) {
	if !f.live {
		return nil, errors.New("port already closed")
	}
	response := make(chan net.Conn)
	f.dial <- dialer{latency: latency, response: response}
	conn := <-response
	if conn != nil {
		return conn, nil
	}
	return nil, errors.New("could not connect")
}

func (t *fakePort) Addr() net.Addr {
	return testAddr(fmt.Sprintf("%v:%v", t.Node.hostname, t.Port))
}

func (f *fakePort) Accept() (net.Conn, error) {
	conn, ok := <-f.accept
	if !ok {
		return nil, errors.New("port already closed")
	}
	return conn, nil
}

func newFakePort(port string, node *testHost) *fakePort {

	listener := fakePort{
		Port:   port,
		Node:   node,
		dial:   make(chan dialer),
		accept: make(chan net.Conn),
		close:  make(chan struct{}, 2),
		live:   true,
	}

	go func() {
		for {
			select {
			case <-listener.close:
				listener.live = false
				close(listener.close)
				close(listener.accept)
				return
			case dial := <-listener.dial:
				if listener.live {
					dialerConn, listenerConn := net.Pipe()
					listenerWithLatency := withLatency(listenerConn, dial.latency+node.latency)
					dialerWithLatency := withLatency(dialerConn, dial.latency+node.latency)
					listener.accept <- listenerWithLatency
					listener.Node.connections = append(listener.Node.connections, listenerWithLatency)
					dial.response <- dialerWithLatency
				} else {
					dial.response <- nil
				}
			}
		}
	}()

	return &listener
}
