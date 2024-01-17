package gateway

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/freehandle/breeze/consensus/admin"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/config"
	"github.com/freehandle/breeze/socket"
)

type fakeRelayNetwork struct {
	events  chan []byte
	actions chan []byte
	gateway []*socket.SignedConnection
	block   []*socket.SignedConnection
	pks     []crypto.PrivateKey
}

func createFakeRelayNetwork(count int) *fakeRelayNetwork {
	fake := &fakeRelayNetwork{
		gateway: make([]*socket.SignedConnection, count),
		block:   make([]*socket.SignedConnection, count),
		pks:     make([]crypto.PrivateKey, count),
	}
	for n := 0; n < count; n++ {
		node := fmt.Sprintf("node%d", +n)
		socket.TCPNetworkTest.AddNode(node, 1, time.Millisecond, 1e9)
		gatewayListener, _ := socket.Listen(fmt.Sprintf("%s:5401", node))
		blockListener, _ := socket.Listen(fmt.Sprintf("%s:5402", node))
		_, fake.pks[n] = crypto.RandomAsymetricKey()
		go func(listener net.Listener, pk crypto.PrivateKey, n int) {
			conn, _ := listener.Accept()
			fake.gateway[n], _ = socket.PromoteConnection(conn, pk, socket.AcceptAllConnections)
		}(gatewayListener, fake.pks[n], n)
		go func(listener net.Listener, pk crypto.PrivateKey, n int) {
			conn, _ := listener.Accept()
			fake.block[n], _ = socket.PromoteConnection(conn, pk, socket.AcceptAllConnections)
			fmt.Println("block connected", n)
		}(blockListener, fake.pks[n], n)
	}
	return fake
}

func TestGateway(t *testing.T) {
	_, pk := crypto.RandomAsymetricKey()
	_, cpk := crypto.RandomAsymetricKey()
	socket.TCPNetworkTest.AddNode("gateway", 1, time.Millisecond, 1e9)
	fake := createFakeRelayNetwork(10)
	config := Configuration{
		Credentials:     pk,
		Wallet:          pk,
		Hostname:        "gateway",
		Port:            5500,
		Firewall:        socket.NewValidConnections(nil, true),
		Trusted:         []socket.TokenAddr{{Addr: "node0", Token: fake.pks[0].PublicKey()}},
		ActionRelayPort: 5401,
		BlockRelayPort:  5402,
		Breeze:          *config.StandardBreezeConfig,
	}
	ctx, cancel := context.WithCancel(context.Background())
	adm := admin.Administration{
		FirewallAction: make(chan admin.FirewallAction),
		Status:         make(chan chan string),
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		fake.block[0].Read()
		topology := &messages.NetworkTopology{
			Start:      1,
			End:        900,
			StartAt:    time.Now(),
			Order:      make([]crypto.Token, 100),
			Validators: make([]socket.TokenAddr, 10),
		}
		for n := 0; n < 10; n++ {
			topology.Validators[n] = socket.TokenAddr{Addr: fmt.Sprintf("node%d", n), Token: fake.pks[n].PublicKey()}
		}
		for n := 0; n < 100; n++ {
			topology.Order[n] = fake.pks[n%10].PublicKey()
		}
		fake.block[0].Send(topology.Serialize())
	}()
	errors := NewServer(ctx, config, &adm)

	//
	time.Sleep(100 * time.Millisecond)
	socket.TCPNetworkTest.AddNode("client", 1, time.Millisecond, 1e9)
	conn, err := socket.Dial("client", "gateway:5500", cpk, pk.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	action := testAction()
	conn.Send(append([]byte{ActionMsg}, action...))
	if data, err := conn.Read(); err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("resposta", data)
	}
	if data, err := fake.gateway[3].Read(); err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("action", data)
	}
	if err := <-errors; err != nil {
		t.Fatal("server failed", err)
	}
	cancel()
}
