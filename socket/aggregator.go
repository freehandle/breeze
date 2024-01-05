package socket

import (
	"context"
	"errors"
	"math/rand"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

// Aggregator consolidates data from multiple providers into a bufferred channel.
// Redundant data is discarded. The aggregator is live until the context is done.
// All connections for Aggregator are initiated by the aggregator itself.
type Aggregator struct {
	live        bool
	hostname    string
	credentials crypto.PrivateKey
	buffer      *util.DataQueue[[]byte]
	providers   []*SignedConnection
	cancel      chan crypto.Token
	add         chan *SignedConnection
	cancelall   chan struct{}
}

// NewAgregator creates a new aggregator. The aggregator is live until the context
// is done. hostname should be empty or localhost for internet connections.
// credentials are used to stablish connections to providers.
func NewAgregator(ctx context.Context, hostname string, credentials crypto.PrivateKey, connections ...*SignedConnection) *Aggregator {
	aggregator := &Aggregator{
		live:        true,
		hostname:    hostname,
		credentials: credentials,
		cancel:      make(chan crypto.Token),
		cancelall:   make(chan struct{}),
		add:         make(chan *SignedConnection),
		buffer:      util.NewDataQueueWithHashFunc(ctx, crypto.Hasher),
	}
	if len(connections) == 0 {
		aggregator.providers = make([]*SignedConnection, 0)
	} else {
		aggregator.providers = connections
	}
	go func() {
		for {
			done := ctx.Done()
			select {
			case <-done:
				for _, provider := range aggregator.providers {
					provider.Shutdown()
				}
				aggregator.buffer.Close()
				aggregator.live = false
				close(aggregator.cancel)
				close(aggregator.add)
				return
			case conn, ok := <-aggregator.add:
				if ok {
					aggregator.providers = append(aggregator.providers, conn)
				}
			case token, ok := <-aggregator.cancel:
				if ok {
					for i, provider := range aggregator.providers {
						if provider.Token.Equal(token) {
							aggregator.providers = append(aggregator.providers[:i], aggregator.providers[i+1:]...)
							provider.Shutdown()
						}
					}
				}
			case <-aggregator.cancelall:
				for _, provider := range aggregator.providers {
					provider.Shutdown()
				}
				aggregator.providers = aggregator.providers[:0]
			}
		}
	}()
	return aggregator
}

// Read returns the next data from the aggregator. It blocks if there is no data
// available.
func (b *Aggregator) Read() ([]byte, error) {
	if !b.live {
		return nil, errors.New("aggregator is not live")
	}
	return b.buffer.Pop(), nil
}

// Has returns true if the aggregator has a connection to the given provider (
// same address and same token) or false otherwise
func (b *Aggregator) Has(peer TokenAddr) bool {
	for _, provider := range b.providers {
		if provider.Token.Equal(peer.Token) {
			return true
		}
	}
	return false
}

// HasAny returns true if the aggregator has a connection to any of the given
// providers or false otherwise
func (b *Aggregator) HasAny(peers []TokenAddr) bool {
	for _, peer := range peers {
		if b.Has(peer) {
			return true
		}
	}
	return false
}

// AddOne will return nil if the aggregator has a connection to any of the given
// peers or it could establish a connection with one of the given peers. It will
// select a random peer to connect to, and if not successful it will try the next
// (in a circular fashion) one until it can connect to one. If it cannot connect
// to any of the given peers, an error is returned.
func (b *Aggregator) AddOne(peers []TokenAddr) error {
	if b.HasAny(peers) {
		return nil
	}
	value := rand.Intn(len(peers))
	for n := 0; n < len(peers); n++ {
		err := b.AddProvider(peers[(value+n)%len(peers)])
		if err == nil {
			return nil
		}
	}
	return errors.New("could not connect to any peer")
}

// AddProvider tries to connect to the given provider and add it to the list of
// providers. If the connection fails, an error is returned.
func (b *Aggregator) AddProvider(provider TokenAddr) error {
	conn, err := Dial(b.hostname, provider.Addr, b.credentials, provider.Token)
	if err != nil {
		return err
	}
	if !b.live {
		conn.Shutdown()
		return errors.New("cannot add provider to non-live blocks")
	} else {
		b.add <- conn
	}
	go func() {
		for {
			data, err := conn.Read()
			if err != nil {
				b.CloseProvider(provider.Token)
				return
			}
			b.buffer.Push(data)
		}
	}()
	return nil
}

// CloseProvider closes the connection to the given provider and exludes it from
// the provider list.
func (b *Aggregator) CloseProvider(provider crypto.Token) {
	if b.live {
		b.cancel <- provider
	}
}

// CloseAllProviders closes all connections to providers and clears the provider
// list.
func (b *Aggregator) Shutdown() {
	if b.live {
		b.cancelall <- struct{}{}
	}
}
