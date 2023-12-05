package swell

import (
	"context"
	"testing"
	"time"

	"github.com/freehandle/breeze/consensus/permission"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/socket"
)

func TestSwell(t *testing.T) {
	socket.TCPNetworkTest.AddNode("node1", 1, 100*time.Millisecond, 1e9)
	ctx, cancel := context.WithCancel(context.Background())
	_, pk := crypto.RandomAsymetricKey()
	config := ValidatorConfig{
		credentials: pk,
		walletPath:  "",
		swellConfig: SwellNetworkConfiguration{
			NetworkHash:      crypto.HashToken(pk.PublicKey()),
			MaxPoolSize:      10,
			MaxCommitteeSize: 100,
			BlockInterval:    time.Second,
			ChecksumWindow:   60,
			Permission:       permission.NewProofOfAuthority(),
		},
		relay:    relay.NewNode(),
		hostname: "node1",
	}

	go func() {
		time.Sleep(1 * time.Second)
		for n := 1; n < 1000; n++ {
			token, _ := crypto.RandomAsymetricKey()
			transfer := actions.Transfer{
				TimeStamp: 1,
				From:      pk.PublicKey(),
				To:        []crypto.TokenValue{{Token: token, Value: 10}},
				Reason:    "Testando o swell",
				Fee:       1,
			}
			transfer.Sign(pk)
			config.relay.ActionGateway <- transfer.Serialize()
		}
		cancel()
	}()
	NewGenesisNode(ctx, pk, config)
}
