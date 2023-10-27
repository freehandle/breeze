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
		channels[n] = BuildCommittee(peers, nil, pk, 3500+n)
		time.Sleep(50 * time.Millisecond)
	}

	first := <-channels[0]
	if len(first) != 2 {
		t.Error("Expected 2 connections, got", len(first))
	}
	second := <-channels[1]
	if len(second) != 2 {
		t.Error("Expected 2 connections, got", len(second))
	}
	third := <-channels[2]
	if len(third) != 2 {
		t.Error("Expected 2 connections, got", len(third))
	}
}
