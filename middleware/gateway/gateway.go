package gateway

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

const SendToNValidators = 5

type ConfigGateway struct {
	Credentials     crypto.PrivateKey
	Wallet          crypto.PrivateKey
	ActionPort      int // receive actions
	NetworkPort     int // receive checksums and send block events
	TrustedProvider socket.TokenAddr
	Hostname        string
}

// Gateway is a very simple implementation of a gateway service for the breeze
// network. It tries to connect to all current validators within the network
// and forward actions to them in accordanco with the swell consensus protocol
// rules. At each time it will send the action to the current proposer and
// the next SendToNValidators validators in the current order. It will also
type Gateway struct {
	mu          sync.Mutex
	credentials crypto.PrivateKey
	wallet      crypto.PrivateKey
	data        chan []byte
	connections map[crypto.Token]*socket.SignedConnection
	provider    *socket.SignedConnection
	trusted     []socket.TokenAddr
	hostname    string
	order       []crypto.Token
}

func (g *Gateway) ConnectTrustedProvider(ctx context.Context, provider socket.TokenAddr) error {
	conn, err := socket.DialCtx(ctx, g.hostname, provider.Addr, g.credentials, provider.Token)
	if err != nil {
		return err
	}
	g.connections[provider.Token] = conn

	return nil
}

func (g *Gateway) updateGateway(order []crypto.Token, validators []socket.TokenAddr) {
	wg := sync.WaitGroup{}
	newConnections := make([]socket.TokenAddr, 0)
	for _, validator := range validators {
		if _, ok := g.connections[validator.Token]; !ok {
			newConnections = append(newConnections, validator)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	wg.Add(len(newConnections))
	for _, member := range newConnections {
		go func(member socket.TokenAddr) {
			conn, err := socket.DialCtx(ctx, g.hostname, member.Addr, g.credentials, member.Token)
			if err == nil {
				g.mu.Lock()
				g.connections[member.Token] = conn
				g.mu.Unlock()
			}
			wg.Done()
		}(member)
	}
	wg.Wait()
	cancel()
}

func (g *Gateway) releaseConnections(validators []socket.TokenAddr) {

}

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
