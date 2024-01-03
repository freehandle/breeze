package admin

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/crypto/dh"
	"github.com/freehandle/breeze/socket"
)

const (
	RequestStatus byte = iota
	EphemeralKey
	DiffieHellman
	InvalidKey
	StatusOk
	StatusErr
	Bye
)

type AdminServer struct {
	mu            sync.Mutex
	Hostname      string
	Firewall      socket.ValidateConnection
	Secret        crypto.PrivateKey
	Port          int
	Status        chan chan string
	DiffieHellman chan crypto.PrivateKey
	live          map[*socket.SignedConnection]struct{}
}

func (a *AdminServer) Start(ctx context.Context) error {
	a.live = make(map[*socket.SignedConnection]struct{})
	fmt.Println("listening", fmt.Sprintf("%s:%v", a.Hostname, a.Port))
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
			trusted, err := socket.PromoteConnection(conn, a.Secret, a.Firewall)
			if err != nil {
				slog.Info("admin connection rejected: %v", err)
				continue
			}
			go a.Panel(trusted)
		}
	}()

	go func() {
		<-withcancel.Done()

	}()

	return nil
}

func (a *AdminServer) Panel(conn *socket.SignedConnection) {
	defer func() {
		a.mu.Lock()
		delete(a.live, conn)
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
		case RequestStatus:
			status := make(chan string)
			a.Status <- status
			response := <-status
			var bytes []byte
			if response != "" {
				bytes = append([]byte{StatusOk}, []byte(response)...)
			} else {
				bytes = []byte{StatusErr}
			}
			if err := conn.Send(bytes); err != nil {
				return
			}
		case DiffieHellman:
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
			a.DiffieHellman <- pk
			if err := conn.Send([]byte{StatusOk}); err != nil {
				return
			}
		case Bye:
			return
		default:
			conn.Send([]byte{StatusErr})
		}
	}
}
