package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/permission"
	"github.com/freehandle/breeze/consensus/poa"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

var config = poa.SingleAuthorityConfig{
	IncomingPort:     5005,
	OutgoingPort:     5006,
	BlockInterval:    time.Second,
	ValidateIncoming: socket.AcceptAllConnections,
	ValidateOutgoing: socket.AcceptAllConnections,
	WalletFilePath:   "", // memory
	KeepBlocks:       50,
}

var pkHex = "f622f274b13993e3f1824a30ef0f7e57f0c35a4fbdc38e54e37916ef06a64a797eb7aa3582b216bba42d45e91e0a560508478f5b55228439b42733945fd5c2f5"

func TestSwell() {
	socket.TCPNetworkTest.AddNode("mainserver", 1, 100*time.Millisecond, 1e9)
	socket.TCPNetworkTest.AddNode("gateway", 1, 100*time.Millisecond, 1e9)
	socket.TCPNetworkTest.AddNode("listener", 1, 100*time.Millisecond, 1e9)
	socket.TCPNetworkTest.AddNode("candidate", 1, 100*time.Millisecond, 1e9)
	ctx, cancel := context.WithCancel(context.Background())
	_, mainserver := crypto.RandomAsymetricKey()
	_, gateway := crypto.RandomAsymetricKey()
	_, listener := crypto.RandomAsymetricKey()
	_, candidate := crypto.RandomAsymetricKey()

	relayConfig := relay.Config{
		GatewayPort:       3030,
		BlockListenerPort: 3031,
		AdminPort:         3032,
		Firewall:          relay.NewFireWall([]crypto.Token{gateway.PublicKey()}, []crypto.Token{listener.PublicKey(), candidate.PublicKey()}),
		Credentials:       mainserver,
		Hostname:          "mainserver",
	}
	relayNode, err := relay.Run(ctx, relayConfig)
	if err != nil {
		log.Fatal(err)
	}
	config := swell.ValidatorConfig{
		Credentials: mainserver,
		WalletPath:  "",
		SwellConfig: swell.SwellNetworkConfiguration{
			NetworkHash:      crypto.HashToken(mainserver.PublicKey()),
			MaxPoolSize:      10,
			MaxCommitteeSize: 100,
			BlockInterval:    time.Second,
			ChecksumWindow:   20,
			Permission:       permission.Permissionless{},
		},
		Relay:    relayNode,
		Hostname: "mainserver",
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer func() {
			wg.Done()
			cancel()
		}()
		time.Sleep(1 * time.Second)
		conn, err := socket.Dial("gateway", "mainserver:3030", gateway, mainserver.PublicKey())
		if err != nil {
			log.Fatal(err)
			return
		}
		for n := 1; n < 3000; n++ {
			time.Sleep(30 * time.Millisecond)
			token, _ := crypto.RandomAsymetricKey()
			transfer := actions.Transfer{
				TimeStamp: 1,
				From:      mainserver.PublicKey(),
				To:        []crypto.TokenValue{{Token: token, Value: 10}},
				Reason:    "Testando o swell",
				Fee:       1,
			}
			transfer.Sign(mainserver)
			if err := conn.Send(append([]byte{messages.MsgActionSubmit}, transfer.Serialize()...)); err != nil {
				fmt.Println(err)
				return
			}
			//fmt.Println("Sent", n)
		}
	}()

	go func() {
		relayConfig := relay.Config{
			GatewayPort:       3030,
			BlockListenerPort: 3031,
			AdminPort:         3032,
			Firewall:          relay.NewFireWall([]crypto.Token{gateway.PublicKey()}, []crypto.Token{}),
			Credentials:       candidate,
			Hostname:          "candidate",
		}
		relayNode, err := relay.Run(ctx, relayConfig)
		if err != nil {
			log.Fatal(err)
		}
		time.Sleep(4 * time.Second)
		provider := socket.TokenAddr{
			Addr:  "mainserver:3031",
			Token: mainserver.PublicKey(),
		}
		config := swell.ValidatorConfig{
			Credentials: candidate,
			WalletPath:  "",
			SwellConfig: swell.SwellNetworkConfiguration{
				NetworkHash:      crypto.HashToken(mainserver.PublicKey()),
				MaxPoolSize:      10,
				MaxCommitteeSize: 100,
				BlockInterval:    time.Second,
				ChecksumWindow:   20,
				Permission:       permission.Permissionless{},
			},
			Relay:    relayNode,
			Hostname: "candidate",
		}
		err = swell.FullSyncValidatorNode(ctx, config, provider)
		if err != nil {
			fmt.Println(err)
		}
	}()

	go func() {
		defer func() {
			wg.Done()
			cancel()
		}()
		time.Sleep(1500 * time.Millisecond)
		conn, err := socket.Dial("listener", "mainserver:3031", listener, mainserver.PublicKey())
		if err != nil || conn == nil {
			log.Print(err)
			return
		}
		request := []byte{messages.MsgSyncRequest}
		util.PutUint64(0, &request)
		util.PutBool(false, &request)
		conn.Send(request)
		for {
			bytes, err := conn.Read()
			if err != nil {
				fmt.Println(err)
				return
			}
			if len(bytes) > 0 {
				if bytes[0] == messages.MsgSealedBlock {
					sealed := chain.ParseSealedBlock(bytes[1:])
					if sealed != nil {
					} else {
						fmt.Println("could not parse sealed block")
					}
				} else if bytes[0] == messages.MsgCommittedBlock {
					commit := chain.ParseCommitBlock(bytes[1:])
					if commit != nil {
					} else {
						fmt.Println("could not parse commit block")
					}
				}
			}
		}
	}()
	swell.NewGenesisNode(ctx, mainserver, config)
	wg.Wait()
}

func TestChannelConn() {

	socket.TCPNetworkTest.AddNode("first", 1, 50*time.Millisecond, 1e9)
	socket.TCPNetworkTest.AddNode("second", 1, 50*time.Millisecond, 1e9)
	listener, err := socket.Listen("first:3500")
	if err != nil {
		log.Fatal(err)
	}
	firstToken, firstPK := crypto.RandomAsymetricKey()
	_, secondPK := crypto.RandomAsymetricKey()
	var s1, s2 *socket.SignedConnection

	go func() {
		fmt.Println("to ouvindo")
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("ouvi")
		s1, err = socket.PromoteConnection(conn, firstPK, socket.AcceptAllConnections)
		fmt.Println("promovido")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("deu certo")
	}()
	time.Sleep(100 * time.Millisecond)
	fmt.Println("to dicando")
	s2, err = socket.Dial("second", "first:3500", secondPK, firstToken)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("conectei")
	time.Sleep(200 * time.Millisecond)
	msg := []byte{1}
	util.PutUint64(10, &msg)
	msg = append(msg, 15, 16, 17)

	c1 := socket.NewChannelConnection(s1)
	c2 := socket.NewChannelConnection(s2)
	data := make(chan []byte)
	c2.Register(10, data)
	c1.Send(msg)
	msgr := <-data
	if len(msgr) != len(msg) {
		log.Fatal("Expected", len(msg), "got", len(msgr))
	}
}

func TestGossip() {
	socket.TCPNetworkTest.AddNode("first", 1, 50*time.Millisecond, 1e9)
	socket.TCPNetworkTest.AddNode("second", 1, 50*time.Millisecond, 1e9)

	firstToken, firstPK := crypto.RandomAsymetricKey()
	secondToken, secondPK := crypto.RandomAsymetricKey()

	first := make(chan []*socket.ChannelConnection)
	second := make(chan []*socket.ChannelConnection)

	peers := []socket.CommitteeMember{
		{Address: "first", Token: firstToken},
		{Address: "second", Token: secondToken},
	}

	go func() {
		first <- socket.AssembleChannelNetwork(context.Background(), peers, firstPK, 3500, "first", nil)
	}()

	go func() {
		second <- socket.AssembleChannelNetwork(context.Background(), peers, secondPK, 3500, "second", nil)
	}()

	conn := make([][]*socket.ChannelConnection, 2)

	conn[0] = <-first
	conn[1] = <-second

	if len(conn[0]) != 1 || len(conn[1]) != 1 {
		log.Fatal("Expected 1, got", len(conn[0]), len(conn[1]))
	}

	if (!conn[0][0].Live) || (!conn[1][0].Live) {
		log.Fatal("Expected live, got", conn[0][0].Live, conn[1][0].Live)
	}

	if !conn[0][0].Is(secondToken) || !conn[1][0].Is(firstToken) {
		log.Fatal("Expected", secondToken, firstToken, "got", conn[0][0].Conn.Token, conn[1][0].Conn.Token)
	}

	time.Sleep(300 * time.Millisecond)

	g1 := socket.GroupGossip(10, conn[0])
	g2 := socket.GroupGossip(10, conn[1])

	msg := []byte{1}
	util.PutUint64(10, &msg)
	msg = append(msg, 15, 16, 17)

	g1.Broadcast(msg)

	msgr := <-g2.Signal
	if len(msgr.Signal) != len(msg) {
		log.Fatal("Expected %v, got %v", len(msg), len(msgr.Signal))
	}

}

func BuildChannelNetwork(pk []crypto.PrivateKey) [][]*socket.ChannelConnection {
	count := len(pk)
	tokens := make([]crypto.Token, count)
	secrets := make([]crypto.PrivateKey, count)
	chanConn := make([][]*socket.ChannelConnection, count)
	peers := make([]socket.CommitteeMember, count)
	for n := 0; n < count; n++ {
		tokens[n] = pk[n].PublicKey()
		secrets[n] = pk[n]
		socket.TCPNetworkTest.AddNode(fmt.Sprintf("n%v", n), 1, 50*time.Millisecond, 1e9)
		peers[n] = socket.CommitteeMember{Address: fmt.Sprintf("n%v", n), Token: tokens[n]}
	}

	wg := sync.WaitGroup{}
	wg.Add(count)
	for n := 0; n < count; n++ {
		go func(pos int) {
			chanConn[pos] = socket.AssembleChannelNetwork(context.Background(), peers, secrets[pos], 3500, fmt.Sprintf("n%v", pos), nil)
			fmt.Println("done", pos)
			wg.Done()
		}(n)
	}
	wg.Wait()
	return chanConn
}
func BuilTestGossipNetwork(pk []crypto.PrivateKey, epoch uint64) []*socket.Gossip {
	count := len(pk)
	chanConn := BuildChannelNetwork(pk)
	gossip := make([]*socket.Gossip, count)
	for n := 0; n < count; n++ {
		gossip[n] = socket.GroupGossip(epoch, chanConn[n])
	}
	return gossip
}

func TestBuildChannel() {
	pk := make([]crypto.PrivateKey, 100)
	for n := 0; n < len(pk); n++ {
		_, pk[n] = crypto.RandomAsymetricKey()
	}
	BuildChannelNetwork(pk)
}

func TestBFT() {
	pk := make([]crypto.PrivateKey, 7)
	for n := 0; n < len(pk); n++ {
		_, pk[n] = crypto.RandomAsymetricKey()
	}
	g := BuilTestGossipNetwork(pk, 1)

	members := make(map[crypto.Token]bft.PoolingMembers)
	order := make([]crypto.Token, 0)
	for n := 0; n < len(pk); n++ {
		members[pk[n].PublicKey()] = bft.PoolingMembers{Weight: 1}
		order = append(order, pk[n].PublicKey())
	}
	hash := crypto.Hasher([]byte("test hash"))
	fmt.Printf("Hash: %v\n\n", hash)
	var wg sync.WaitGroup
	wg.Add(len(pk))
	for n := 0; n < len(pk); n++ {
		go func(n int, credentials crypto.PrivateKey) {
			p := bft.PoolingCommittee{
				Epoch:   1,
				Members: members,
				Order:   order,
				Gossip:  g[n],
			}
			pools := bft.LaunchPooling(p, credentials)
			if n == 0 {
				go func() {
					time.Sleep(400 * time.Millisecond)
					pools.SealBlock(hash, credentials.PublicKey())
				}()
			}
			consensus := <-pools.Finalize
			wg.Done()
			if !consensus.Value.Equal(hash) {
				log.Fatal("Expected", hash, "got", consensus.Value)
			}
		}(n, pk[n])
	}
	wg.Wait()
}

func main() {
	TestSwell()
	/*bytes, _ := hex.DecodeString(pkHex)
	var pk crypto.PrivateKey
	copy(pk[:], bytes)
	tokenBytes, _ := hex.DecodeString(pkHex[64:])
	var token crypto.Token
	copy(token[:], tokenBytes)
	if !pk.PublicKey().Equal(token) {
		log.Fatalf("invalid credentials")
	}
	config.Credentials = pk
	err := <-poa.Genesis(config)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("done")
	}*/
}
