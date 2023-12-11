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

// An implementation with ValidateConnection interface that accepts all reequested
// connections.
var AcceptAllConnections = acceptAll{}

type acceptAll struct{}

// ValidateConnection returns a channel with value true.
func (a acceptAll) ValidateConnection(token crypto.Token) chan bool {
	response := make(chan bool, 2)
	response <- true
	return response
}

// An implementation with ValidateConnection interface that accepts only
// connection from a single token.
type ValidateSingleConnection crypto.Token

// ValidateConnection returns a channel with true if the given token is equal to
// the token assocated with ValidateSingleConnection and false otherwise.
func (v ValidateSingleConnection) ValidateConnection(token crypto.Token) chan bool {
	response := make(chan bool, 2)
	response <- token.Equal(crypto.Token(v))
	return response
}

// An implementation with ValidateConnection interface that accepts only
// connections from a list of tokens.
type AcceptValidConnections struct {
	mu    sync.Mutex
	valid []crypto.Token
}

// NewValidConnections returns a new AcceptValidConnections with the given
// list of tokens.
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

// Add adds a token to the list of valid tokens.
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

// Remove removes a token from the list of valid tokens.
func (a *AcceptValidConnections) Remove(token crypto.Token) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for n, valid := range a.valid {
		if valid.Equal(token) {
			a.valid = append(a.valid[:n], a.valid[n+1:]...)
		}
	}
}

// ValidateConnection returns channled with value true if the given token is in
// the list of valid tokens and false otherwise.
func (a *AcceptValidConnections) ValidateConnection(token crypto.Token) chan bool {
	response := make(chan bool, 2)
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, valid := range a.valid {
		if valid.Equal(token) {
			response <- true
			return response
		}
	}
	response <- false
	return response
}
