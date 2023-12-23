package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

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
						fmt.Println("could not parse sealed block")
					}
				}
			}
		}
	}()
	swell.NewGenesisNode(ctx, mainserver, config)
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
