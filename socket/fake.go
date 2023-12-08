package socket

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type testAddr string

func (t testAddr) Network() string {
	return "tcp"
}

func (t testAddr) String() string {
	return string(t)
}

type testMessage struct {
	when time.Time
	data []byte
}

type testConn struct {
	once    sync.Once
	write   chan []byte
	latency time.Duration
	conn    net.Conn
}

func (t *testConn) Read(data []byte) (int, error) {
	return t.conn.Read(data)
}

func (t *testConn) Write(data []byte) (int, error) {
	t.write <- data
	return len(data), nil
}

func (t *testConn) Close() error {
	t.once.Do(func() {
		close(t.write)
	})
	return t.conn.Close()
}

func (t *testConn) LocalAddr() net.Addr {
	return t.conn.LocalAddr()
}

func (t *testConn) RemoteAddr() net.Addr {
	return t.conn.RemoteAddr()
}

func (t *testConn) SetDeadline(d time.Time) error {
	return t.conn.SetDeadline(d)
}

func (t *testConn) SetReadDeadline(d time.Time) error {
	return t.conn.SetReadDeadline(d)
}

func (t *testConn) SetWriteDeadline(d time.Time) error {
	return t.conn.SetWriteDeadline(d.Add(-t.latency))
}

func withLatency(conn net.Conn, latency time.Duration) net.Conn {
	test := testConn{
		write: make(chan []byte),
		conn:  conn,
	}

	latencyWrite := make([]testMessage, 0)
	received := make(chan []byte)
	go func() {
		for {
			data, ok := <-received
			if !ok {
				return
			}
			conn.Write(data)
		}
	}()

	go func() {
		timer := time.NewTimer(time.Hour)
		for {
			select {
			case data, ok := <-test.write:
				if !ok {
					close(received)
					return
				}
				msg := testMessage{
					when: time.Now().Add(latency),
					data: data,
				}
				if len(latencyWrite) == 0 {
					latencyWrite = []testMessage{msg}
					timer.Reset(latency)
				} else {
					latencyWrite = append(latencyWrite, msg)
				}
			case <-timer.C:
				if len(latencyWrite) > 0 {
					received <- latencyWrite[0].data
					latencyWrite = latencyWrite[1:]
					if len(latencyWrite) > 0 {
						timer.Reset(time.Until(latencyWrite[0].when))
					}
				}
			}
		}
	}()
	return &test
}

type testHost struct {
	hostname      string
	reliability   float64
	latency       time.Duration
	maxThroughput int
	connections   []net.Conn
	network       *testNetwork
}

type testPort struct {
	Port   string
	Node   *testHost
	accept chan net.Conn
}

func (t *testNetwork) Dial(hostname, address string) (net.Conn, error) {
	host, ok := t.hosts[hostname]
	if !ok {
		return nil, errors.New("testiing hostname not registered")
	}
	for n := 0; n < 3; n++ {
		if listener, ok := t.listeners[address]; ok {
			dialerConn, listenerConn := net.Pipe()
			listenetWithLatency := withLatency(listenerConn, host.latency+listener.Node.latency)
			listener.accept <- listenetWithLatency
			listener.Node.connections = append(listener.Node.connections, listenetWithLatency)
			dialerWithLatency := withLatency(dialerConn, host.latency+listener.Node.latency)
			host.connections = append(host.connections, dialerWithLatency)
			return dialerWithLatency, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("server did not respond")
}

func (t *testPort) Close() error {
	delete(t.Node.network.listeners, fmt.Sprintf("%v:%v", t.Node.hostname, t.Port))
	return nil
}

func (t *testPort) Addr() net.Addr {
	return testAddr(fmt.Sprintf("%v:%v", t.Node.hostname, t.Port))
}

func (t *testPort) Accept() (net.Conn, error) {
	conn, ok := <-t.accept
	if !ok {
		return nil, errors.New("listener closed")
	}
	return conn, nil
}

type testNetwork struct {
	hosts     map[string]*testHost
	ctx       context.Context
	listeners map[string]*testPort
	live      []net.Conn
}

var TCPNetworkTest = &testNetwork{
	hosts:     make(map[string]*testHost),
	ctx:       context.Background(),
	listeners: make(map[string]*testPort),
	live:      make([]net.Conn, 0),
}

func (t *testNetwork) Listen(address string) (net.Listener, error) {
	hostname, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	host, ok := t.hosts[hostname]
	if !ok {
		return nil, errors.New("node not found")
	}
	if _, ok := t.listeners[address]; ok {
		return nil, errors.New("port already in use")
	}
	test := &testPort{
		Port:   port,
		Node:   host,
		accept: make(chan net.Conn),
	}
	t.listeners[address] = test
	return test, nil

}

func (n *testNetwork) AddNode(hostname string, Reliability float64, Latency time.Duration, MaxThroughput int) {
	if hostname == "" || hostname == "localhost" {
		panic("testNetwork invalid hostname")
	}
	host := testHost{
		hostname:      hostname,
		reliability:   Reliability,
		latency:       Latency,
		maxThroughput: MaxThroughput,
		connections:   make([]net.Conn, 0),
		network:       n,
	}
	n.hosts[hostname] = &host
}

func (n *testNetwork) AddReliableNode(address string, Latency time.Duration, MaxThroughput int) {
	n.AddNode(address, 1, Latency, MaxThroughput)
}
