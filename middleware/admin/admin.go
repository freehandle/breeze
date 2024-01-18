package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/crypto/dh"
	"github.com/freehandle/breeze/socket"
)

type Activation struct {
	Active   bool
	Response chan bool
}

const (
	InvalidScope byte = iota
	GrantGateway
	RevokeGateway
	GrantBlockListener
	RevokeBlockListener
)

const (
	Instruction byte = iota
	MsgAdminReport
	MsgActivation
	MsgShutdown
	EphemeralKey
	DiffieHellman
	FirewallInstruction
	InvalidKey
	StatusOk
	StatusErr
	Bye
)

type FirewallAction struct {
	Scope byte
	Token crypto.Token
}

func FirewallActionMessage(scope byte, token crypto.Token) []byte {
	return append([]byte{FirewallInstruction, scope}, token[:]...)
}

func ParseFirewallActionMessage(msg []byte) FirewallAction {
	action := FirewallAction{
		Scope: InvalidScope,
		Token: crypto.ZeroToken,
	}
	if len(msg) != 2+crypto.TokenSize || msg[0] != FirewallInstruction {
		return action
	}
	copy(action.Token[:], msg[2:])
	action.Scope = msg[1]
	return action
}

func (a FirewallAction) Grant() bool {
	return a.Scope%2 == 1
}

func (a FirewallAction) Gateway() bool {
	return a.Scope == GrantGateway || a.Scope == RevokeGateway
}

type Interaction struct {
	Request  []byte
	Response chan []byte
}

type Administration struct {
	mu              sync.Mutex
	Hostname        string
	AdmFirewall     *socket.AcceptValidConnections
	Secret          crypto.PrivateKey
	Port            int
	Interaction     chan Interaction
	GatewayFirewall *socket.AcceptValidConnections
	BlockFirewall   *socket.AcceptValidConnections
	live            map[*socket.SignedConnection]struct{}
	diffieHellman   chan crypto.PrivateKey
	hasSyncedKey    bool
	isRunning       bool
}

func (a *Administration) WaitForKeys(ctx context.Context, token crypto.Token) (crypto.PrivateKey, error) {
	a.diffieHellman = make(chan crypto.PrivateKey)
	a.hasSyncedKey = false
	err := a.RunServer(ctx)
	if err != nil {
		return crypto.ZeroPrivateKey, fmt.Errorf("could not start admin server: %v", err)
	}
	pk := <-a.diffieHellman
	if pk.PublicKey().Equal(token) {
		a.Secret = pk
		return pk, nil
	}
	return crypto.ZeroPrivateKey, errors.New("could not retrieve valid secret key")
}

func OpenAdminPort(ctx context.Context, hostname string, credentials crypto.PrivateKey, port int, gateway, blocks *socket.AcceptValidConnections) (*Administration, error) {
	adm := &Administration{
		Hostname:        hostname,
		AdmFirewall:     socket.NewValidConnections([]crypto.Token{credentials.PublicKey()}, false),
		Secret:          credentials,
		Port:            port,
		Interaction:     make(chan Interaction),
		GatewayFirewall: socket.NewValidConnections([]crypto.Token{}, false),
		BlockFirewall:   socket.NewValidConnections([]crypto.Token{}, false),
		live:            make(map[*socket.SignedConnection]struct{}),
	}
	err := adm.RunServer(ctx)
	if err != nil {
		return nil, err
	}
	return adm, nil
}

func (a *Administration) RunServer(ctx context.Context) error {
	a.live = make(map[*socket.SignedConnection]struct{})
	listener, err := socket.Listen(fmt.Sprintf("%s:%v", a.Hostname, a.Port))
	if err != nil {
		return err
	}
	withcancel, cancel := context.WithCancel(ctx)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				slog.Info("admin listener error: %v", err)
				cancel()
				return
			}
			trusted, err := socket.PromoteConnection(conn, a.Secret, a.AdmFirewall)
			if err != nil {
				slog.Info("admin connection rejected", "error", err, "token", a.Secret.PublicKey())
				continue
			}
			go a.Panel(trusted)
		}
	}()

	go func() {
		<-withcancel.Done()
		a.mu.Lock()
		for conn := range a.live {
			conn.Shutdown()
		}
		a.mu.Unlock()
	}()

	return nil
}

func (a *Administration) Panel(conn *socket.SignedConnection) {
	defer func() {
		a.mu.Lock()
		delete(a.live, conn)
		a.mu.Unlock()
		conn.Shutdown()
	}()
	a.mu.Lock()
	a.live[conn] = struct{}{}
	a.mu.Unlock()
	ephPK, eph := dh.NewEphemeralKey()
	if err := conn.Send(append([]byte{EphemeralKey}, eph[:]...)); err != nil {
		return
	}
	for {
		data, err := conn.Read()
		if err != nil {
			return
		}
		if len(data) < 1 {
			continue
		}
		switch data[0] {
		case Instruction:
			response := make(chan []byte)
			a.Interaction <- Interaction{
				Request:  data[1:],
				Response: response,
			}
			resp := <-response
			if len(resp) == 0 {
				conn.Send([]byte{StatusErr})
			} else {
				conn.Send(append([]byte{StatusOk}, resp...))
			}
		case DiffieHellman:
			if a.hasSyncedKey {
				if err := conn.Send([]byte{StatusOk}); err != nil {
					return
				}
				continue
			}
			key := data[1:]
			if len(key) < crypto.TokenSize {
				if err := conn.Send([]byte{InvalidKey}); err != nil {
					return
				}
				continue
			}
			var token crypto.Token
			copy(token[:], key[0:crypto.TokenSize])
			cipher := dh.ConsensusCipher(ephPK, token)
			secret, err := cipher.Open(key[crypto.TokenSize:])
			if err != nil || !crypto.IsValidPrivateKey(secret) {
				if err := conn.Send([]byte{InvalidKey}); err != nil {
					return
				}
			}
			var pk crypto.PrivateKey
			copy(pk[:], secret)
			a.diffieHellman <- pk
			if err := conn.Send([]byte{StatusOk}); err != nil {
				return
			}
			a.hasSyncedKey = true
			a.isRunning = true
		case FirewallInstruction:
			ok := true
			action := ParseFirewallActionMessage(data)
			switch action.Scope {
			case GrantGateway:
				a.GatewayFirewall.Add(action.Token)
			case RevokeGateway:
				a.GatewayFirewall.Remove(action.Token)
			case GrantBlockListener:
				a.BlockFirewall.Add(action.Token)
			case RevokeBlockListener:
				a.BlockFirewall.Remove(action.Token)
			default:
				ok = false
			}
			if ok {
				conn.Send([]byte{StatusOk})
			} else {
				conn.Send([]byte{StatusErr})
			}
		case Bye:
			return
		default:
			conn.Send([]byte{StatusErr})
		}
	}
}
