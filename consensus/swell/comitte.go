package swell

import (
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type CommitteeMemeber struct {
	Address string
	Token   crypto.Token
	Weight  int
}

type Committee struct {
	Start uint64

	Peers []*socket.SignedConnection
	Block *chain.BlockBuilder
}
