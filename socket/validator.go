package socket

import (
	"sync"

	"github.com/freehandle/breeze/crypto"
)

// ValidateConnection is an interface used by handshake protocol to confirm if
// a given token is accredited with rights to establish the connection.
type ValidateConnection interface {
	ValidateConnection(token crypto.Token) chan bool
}

type acceptAll struct{}

func (a acceptAll) ValidateConnection(token crypto.Token) chan bool {
	response := make(chan bool)
	go func() {
		response <- true
	}()
	return response
}

type ValidateSingleConnection crypto.Token

func (v ValidateSingleConnection) ValidateConnection(token crypto.Token) chan bool {
	response := make(chan bool)
	go func() {
		response <- token.Equal(crypto.Token(v))
	}()
	return response
}

// An implementation with ValidateConnection interface that accepts all reequested
// connections.
var AcceptAllConnections = acceptAll{}

func NewValidConnections(conn []crypto.Token) *AcceptValidConnections {
	if len(conn) == 0 {
		return &AcceptValidConnections{
			mu:    sync.Mutex{},
			valid: make([]crypto.Token, 0),
		}
	}
	return &AcceptValidConnections{
		mu:    sync.Mutex{},
		valid: conn,
	}
}

type AcceptValidConnections struct {
	mu    sync.Mutex
	valid []crypto.Token
}

func (a *AcceptValidConnections) Add(token crypto.Token) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, valid := range a.valid {
		if valid.Equal(token) {
			return
		}
	}
	a.valid = append(a.valid, token)
}

func (a *AcceptValidConnections) Remove(token crypto.Token) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for n, valid := range a.valid {
		if valid.Equal(token) {
			a.valid = append(a.valid[:n], a.valid[n+1:]...)
		}
	}
}

func (a *AcceptValidConnections) ValidateConnection(token crypto.Token) chan bool {
	reponse := make(chan bool)
	go func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, valid := range a.valid {
			if valid.Equal(token) {
				reponse <- true
				return
			}
		}
		reponse <- false
	}()
	return reponse
}
