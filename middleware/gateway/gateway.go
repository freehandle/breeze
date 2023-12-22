package gateway

import (
	"context"
	"errors"
	"sync"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type ConfigGateway struct {
	Credentials     crypto.PrivateKey
	Wallet          crypto.PrivateKey
	ActionPort      int // receive actions
	NetworkPort     int // receive checksums and send block events
	TrustedProvider socket.TokenAddr
	Hostname        string
}

type Gateway struct {
	mu          sync.Mutex
	credentials crypto.PrivateKey
	wallet      crypto.PrivateKey
	data        chan []byte
	connections map[crypto.Token]*socket.SignedConnection
	hostname    string
	order       []crypto.Token
}

func (g *Gateway) updateGateway(order []crypto.Token, validators []socket.CommitteeMember) {
	wg := sync.WaitGroup{}
	newConnections := make([]socket.CommitteeMember, 0)
	for _, validator := range validators {
		if _, ok := g.connections[validator.Token]; !ok {
			newConnections = append(newConnections, validator)
		}
	}
	wg.Add(len(newConnections))
	for _, member := range newConnections {
		go func(member socket.CommitteeMember) {
			conn, err := socket.Dial(g.hostname, member.Address, g.credentials, member.Token)
			if err == nil {
				g.mu.Lock()
				g.connections[member.Token] = conn
				g.mu.Unlock()
			}
			wg.Done()
		}(member)
	}
	wg.Wait()
}

func (g *Gateway) releaseConnections(validators []socket.CommitteeMember)

func NewConfigGateway(ctx context.Context, config ConfigGateway) chan error {
	finalize := make(chan error, 2)
	provider, err := socket.Dial(config.Hostname, config.TrustedProvider.Addr, config.Credentials, config.TrustedProvider.Token)
	if err != nil {
		finalize <- err
		return finalize
	}
	provider.Send([]byte{messages.MsgNetworkTopologyReq})
	msg, err := provider.Read()
	if err != nil {
		finalize <- errors.New("could not retrieve network topology from provider")
		return finalize
	}
	order, validators := swell.ParseCommitee(msg)
	if len(order) < 1 || len(validators) < 1 {
		finalize <- errors.New("could not retrieve valid network topology from provider")
		return finalize
	}
	return finalize
}
