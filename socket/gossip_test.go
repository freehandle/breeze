package socket

import (
	"context"
	"testing"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

func TestBuildGossip(t *testing.T) {

	TCPNetworkTest.AddNode("first", 1, 100*time.Millisecond, 1e9)
	TCPNetworkTest.AddNode("second", 1, 100*time.Millisecond, 1e9)

	first := make(chan []*ChannelConnection)
	second := make(chan []*ChannelConnection)

	firstToken, firstPK := crypto.RandomAsymetricKey()
	secondToken, secondPK := crypto.RandomAsymetricKey()

	peers := []CommitteeMember{
		{Address: "first", Token: firstToken},
		{Address: "second", Token: secondToken},
	}

	go func() {
		first <- AssembleChannelNetwork(context.Background(), peers, firstPK, 3500, "first", nil)
	}()

	go func() {
		second <- AssembleChannelNetwork(context.Background(), peers, secondPK, 3500, "second", nil)
	}()

	conn := make([][]*ChannelConnection, 2)

	conn[0] = <-first
	conn[1] = <-second

	if len(conn[0]) != 1 || len(conn[1]) != 1 {
		t.Error("Expected 1, got", len(conn[0]), len(conn[1]))
	}

	if (!conn[0][0].Live) || (!conn[1][0].Live) {
		t.Error("Expected live, got", conn[0][0].Live, conn[1][0].Live)
	}

	if !conn[0][0].Is(secondToken) || !conn[1][0].Is(firstToken) {
		t.Error("Expected", secondToken, firstToken, "got", conn[0][0].Conn.Token, conn[1][0].Conn.Token)
	}

	conn[0][0].Activate()
	conn[1][0].Activate()

	g1 := GroupGossip(10, conn[0])
	g2 := GroupGossip(10, conn[1])

	msg := []byte{1}
	util.PutUint64(10, &msg)
	msg = append(msg, 15, 16, 17)

	g1.Broadcast(msg)

	msgr := <-g2.Signal
	if len(msgr.Signal) != len(msg) {
		t.Errorf("Expected %v, got %v", len(msg), len(msgr.Signal))
	}

}
