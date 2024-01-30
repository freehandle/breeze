package socket

import (
	"fmt"
	"sync"

	"github.com/freehandle/breeze/crypto"
)

// ValidateConnection is an interface used by handshake protocol to confirm if
// a given token is accredited with rights to establish the connection.
type ValidateConnection interface {
	ValidateConnection(token crypto.Token) chan bool
	String() string
}

// An implementation with ValidateConnection interface that accepts all reequested
// connections.
var AcceptAllConnections = acceptAll{}

type acceptAll struct{}

func (a acceptAll) String() string {
	return "accept all connections"
}

// ValidateConnection returns a channel with value true.
func (a acceptAll) ValidateConnection(token crypto.Token) chan bool {
	response := make(chan bool, 2)
	response <- true
	return response
}

// An implementation with ValidateConnection interface that accepts only
// connection from a single token.
type ValidateSingleConnection crypto.Token

func (v ValidateSingleConnection) String() string {
	return fmt.Sprintf("accept only connection from %v", crypto.Token(v))
}

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
	mu   sync.Mutex
	list []crypto.Token
	open bool
}

func (a *AcceptValidConnections) String() string {
	tokenList := ""
	for _, token := range a.list {
		tokenList = fmt.Sprintf("%v%v ", tokenList, token)
	}
	if a.open {
		return fmt.Sprintf("accept all connections except %v", tokenList)
	} else {
		return fmt.Sprintf("block all connections except %v", tokenList)
	}
}

// NewValidConnections returns a new AcceptValidConnections with the given
// list of tokens.
func NewValidConnections(conn []crypto.Token, open bool) *AcceptValidConnections {
	if len(conn) == 0 {
		return &AcceptValidConnections{
			mu:   sync.Mutex{},
			list: make([]crypto.Token, 0),
			open: open,
		}
	}
	return &AcceptValidConnections{
		mu:   sync.Mutex{},
		list: conn,
		open: open,
	}
}

// Add adds a token to the list of valid tokens.
func (a *AcceptValidConnections) Add(token crypto.Token) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.open {
		// exclude from blacklist if open by default
		for n, black := range a.list {
			if black.Equal(token) {
				a.list = append(a.list[:n], a.list[n+1:]...)
				return
			}
		}
	} else {
		// include in whitelist if closed by default
		for _, white := range a.list {
			if white.Equal(token) {
				return
			}
		}
		a.list = append(a.list, token)
	}

}

// Remove removes a token from the list of valid tokens.
func (a *AcceptValidConnections) Remove(token crypto.Token) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.open {
		// include on blacklist if open by default
		for _, black := range a.list {
			if black.Equal(token) {
				return
			}
		}
		a.list = append(a.list, token)
	} else {
		// exclude from white if closed by default
		for n, white := range a.list {
			if white.Equal(token) {
				a.list = append(a.list[:n], a.list[n+1:]...)
				return
			}
		}
	}
}

// ValidateConnection returns channled with value true if the given token is in
// the list of valid tokens and false otherwise.
func (a *AcceptValidConnections) ValidateConnection(token crypto.Token) chan bool {
	response := make(chan bool, 2)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.open {
		for _, black := range a.list {
			if black.Equal(token) {
				fmt.Println("rejecting blacklisted connection", token)
				response <- false
				return response
			}
		}
		fmt.Println("accepting nonblacklisted connection", token)
		response <- true
		return response
	} else {
		for _, white := range a.list {
			if white.Equal(token) {
				fmt.Println("accepting whitelisted connection", token)
				response <- true
				return response
			}
		}
		fmt.Println("rejecting nonwhitelisted connection", token)
		response <- false
		return response
	}
}
