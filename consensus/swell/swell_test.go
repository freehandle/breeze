package swell

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/permission"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

func TestSwell(t *testing.T) {
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
		t.Fatal(err)
	}
	config := ValidatorConfig{
		credentials: mainserver,
		walletPath:  "",
		swellConfig: SwellNetworkConfiguration{
			NetworkHash:      crypto.HashToken(mainserver.PublicKey()),
			MaxPoolSize:      10,
			MaxCommitteeSize: 100,
			BlockInterval:    time.Second,
			ChecksumWindow:   20,
			Permission:       permission.NewProofOfAuthority(),
		},
		relay:    relayNode,
		hostname: "mainserver",
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
			t.Error(err)
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
			if err := conn.Send(append([]byte{chain.MsgActionSubmit}, transfer.Serialize()...)); err != nil {
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
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
		provider := socket.TokenAddr{
			Addr:  "mainserver:3031",
			Token: mainserver.PublicKey(),
		}
		config := ValidatorConfig{
			credentials: candidate,
			walletPath:  "",
			swellConfig: SwellNetworkConfiguration{
				NetworkHash:      crypto.HashToken(mainserver.PublicKey()),
				MaxPoolSize:      10,
				MaxCommitteeSize: 100,
				BlockInterval:    time.Second,
				ChecksumWindow:   20,
				Permission:       permission.NewProofOfAuthority(),
			},
			relay:    relayNode,
			hostname: "candidate",
		}
		err = FullSyncValidatorNode(ctx, config, provider)
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
			t.Error(err)
			return
		}
		request := []byte{chain.MsgSyncRequest}
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
				if bytes[0] == chain.MsgBlockSealed {
					sealed := chain.ParseSealedBlock(bytes[1:])
					if sealed != nil {
					} else {
						fmt.Println("could not parse sealed block")
					}
				} else if bytes[0] == chain.MsgBlockCommitted {
					commit := chain.ParseCommitBlock(bytes[1:])
					if commit != nil {
					} else {
						fmt.Println("could not parse sealed block")
					}
				}
			}
		}
	}()
	NewGenesisNode(ctx, mainserver, config)
	wg.Wait()
}
