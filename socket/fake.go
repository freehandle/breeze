package socket

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

// testAddr implemnets a Network interface for testing purposes
type testAddr string

// Network returns "tcp""
func (t testAddr) Network() string {
	return "tcp"
}

// String returns the name of the test address
func (t testAddr) String() string {
	return string(t)
}

// testMessage is a timestamped message for testing
type testMessage struct {
	when time.Time
	data []byte
}

// testConn implements a net.Conn for testing purposes with latency logic
type testConn struct {
	once    sync.Once
	write   chan []byte
	latency time.Duration
	conn    net.Conn
}

// Read reads data from the test connection
func (t *testConn) Read(data []byte) (int, error) {
	return t.conn.Read(data)
}

// Write writes data to the test connection
func (t *testConn) Write(data []byte) (int, error) {
	t.write <- data
	return len(data), nil
}

// Close closes the test connection
func (t *testConn) Close() error {
	t.once.Do(func() {
		close(t.write)
	})
	return t.conn.Close()
}

// LocalAddr returns the local address of the test connection
func (t *testConn) LocalAddr() net.Addr {
	return t.conn.LocalAddr()
}

// RemoteAddr returns the remote address of the test connection
func (t *testConn) RemoteAddr() net.Addr {
	return t.conn.RemoteAddr()
}

// SetDeadline sets the deadline of the test connection
func (t *testConn) SetDeadline(d time.Time) error {
	return t.conn.SetDeadline(d)
}

// SetReadDeadline sets the read deadline of the test connection
func (t *testConn) SetReadDeadline(d time.Time) error {
	return t.conn.SetReadDeadline(d)
}

// SetWriteDeadline sets the write deadline of the test connection
func (t *testConn) SetWriteDeadline(d time.Time) error {
	return t.conn.SetWriteDeadline(d.Add(-t.latency))
}

// withLatency adds latency to a given net.Conn connection.
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

// testHost implements a net.Listener for testing purposes. Several ports can
// share the same hostname. Hostname cannot bem "localhost" or empty
type testHost struct {
	hostname      string
	reliability   float64
	latency       time.Duration
	maxThroughput int
	connections   []net.Conn
	network       *testNetwork
}

// testPort implementar a port on a testHost for testing purposes. It accepts
// connections a that port.
/*type testPort struct {
	mu     sync.Mutex
	Port   string
	Node   *testHost
	accept chan net.Conn
	Live   bool
}

func (t *testPort) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.Live {
		return errors.New("port already closed")
	}
	delete(t.Node.network.listeners, fmt.Sprintf("%v:%v", t.Node.hostname, t.Port))
	close(t.accept)
	t.Live = false
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
*/

// testNetwork defines a local network for testing purposes. all hosts must be
// declared before usage.
type testNetwork struct {
	hosts map[string]*testHost
	ctx   context.Context
	//listeners map[string]*testPort
	listeners map[string]*fakePort
	live      []net.Conn
}

// TCPNetworkTest is the global test network
var TCPNetworkTest = &testNetwork{
	hosts:     make(map[string]*testHost),
	ctx:       context.Background(),
	listeners: make(map[string]*fakePort),
	live:      make([]net.Conn, 0),
}

// Dial dials a given address on behalf of the hostname. It returns an error
// if the address is not registered as a listener or if the hostname is not
// registered as a host.
func (t *testNetwork) Dial(hostname, address string) (net.Conn, error) {
	host, ok := t.hosts[hostname]
	if !ok {
		return nil, errors.New("testiing hostname not registered")
	}
	for n := 0; n < 3; n++ {
		if listener, ok := t.listeners[address]; ok {
			conn, err := listener.Connect(host.latency)
			if err == nil {
				host.connections = append(host.connections, conn)
				return conn, nil
			}
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("server did not respond")
}

// Listen registered a listener on a hostname:port address. It returns an error
// if the hostname is not registered as a host or if the port is already in use.
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
	test := newFakePort(port, host)
	t.listeners[address] = test
	return test, nil

}

// AddNode register a new node on the test network. The hostname cannot be
// "localhost" or empty. Realiability is a value between 0 and 1, where 1 is
// 100% reliable. Latency is the latency of the connection. MaxThroughput is
// the maximum number of bytes per second that the node can send.
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

// AddReliableNode register a new node on the test network. The hostname cannot
// "localhost" or empty. Latency is the latency of the connection. MaxThroughput
// is the maximum number of bytes per second that the node can send. The node
// is 100% reliable.
func (n *testNetwork) AddReliableNode(address string, Latency time.Duration, MaxThroughput int) {
	n.AddNode(address, 1, Latency, MaxThroughput)
}
