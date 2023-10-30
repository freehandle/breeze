package socket

import (
	"testing"
	"time"

	"github.com/freehandle/breeze/crypto"
)

func TestBuildCommittee(t *testing.T) {

	firstToken, firstPK := crypto.RandomAsymetricKey()
	secondToken, secondPK := crypto.RandomAsymetricKey()
	thirdToken, thirdPK := crypto.RandomAsymetricKey()

	peers := []CommitteeMember{
		{Address: "localhost:3500", Token: firstToken},
		{Address: "localhost:3501", Token: secondToken},
		{Address: "localhost:3502", Token: thirdToken},
	}

	credentials := []crypto.PrivateKey{firstPK, secondPK, thirdPK}
	channels := make([]chan []*ChannelConnection, 3)
	for n, pk := range credentials {
		channels[n] = AssembleCommittee(peers, nil, NewChannelConnection, pk, 3500+n)
		time.Sleep(50 * time.Millisecond)
	}

	all := [][]*ChannelConnection{make([]*ChannelConnection, 3), make([]*ChannelConnection, 3), make([]*ChannelConnection, 3)}

	conns := <-channels[0]
	if len(conns) != 2 {
		t.Error("Expected 2 connections, got", len(conns))
	}
	if conns[0].Conn.Token.Equal(secondToken) {
		all[0] = []*ChannelConnection{nil, conns[0], conns[1]}
	} else {
		all[0] = []*ChannelConnection{nil, conns[1], conns[0]}
	}
	conns[0].Activate()
	conns[1].Activate()

	conns = <-channels[1]
	if len(conns) != 2 {
		t.Error("Expected 2 connections, got", len(conns))
	}
	if conns[0].Conn.Token.Equal(firstToken) {
		all[1] = []*ChannelConnection{conns[0], nil, conns[1]}
	} else {
		all[1] = []*ChannelConnection{conns[1], nil, conns[0]}
	}
	conns[0].Activate()
	conns[1].Activate()

	conns = <-channels[2]
	if len(conns) != 2 {
		t.Error("Expected 2 connections, got", len(conns))
	}
	if conns[0].Conn.Token.Equal(firstToken) {
		all[2] = []*ChannelConnection{conns[0], conns[1], nil}
	} else {
		all[2] = []*ChannelConnection{conns[1], conns[0], nil}
	}
	conns[0].Activate()
	conns[1].Activate()

	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if i != j {
				all[i][j].Send([]byte{12, 13})
				bytes := all[j][i].Read()
				if len(bytes) != 2 || bytes[0] != 12 || bytes[1] != 13 {
					t.Error("Expected to receive [12, 13], got", bytes)
				}

			}
		}
	}
}
