package simple

import (
	"context"

	"fmt"
	"net"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/social"
	"github.com/freehandle/breeze/socket"
)

type SimpleBlock struct {
	Epoch   uint64
	Actions [][]byte
}

type SimpleChain[M social.Merger[M], B social.Blocker[M]] struct {
	Interval    time.Duration
	GatewayPort int
	Writer      *SimpleBlockWriter
	State       social.Stateful[M, B]
	Epoch       uint64
	Recent      [][][]byte
	Keep        int
}

func Gateway(ctx context.Context, port int, token crypto.Token, credentials crypto.PrivateKey) (chan []byte, error) {
	gateway, err := socket.Dial("localhost", fmt.Sprintf(":%d", port), credentials, token)
	if err != nil {
		return nil, err
	}
	receiver := make(chan []byte, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				gateway.Shutdown()
				return
			case data := <-receiver:
				err := gateway.Send(data)
				if err != nil {
					gateway.Shutdown()
					return
				}
			}
		}
	}()
	return receiver, nil
}

func (sc *SimpleChain[M, B]) Start(ctx context.Context, credentials crypto.PrivateKey, logger func([]byte) string) chan error {
	ticker := time.NewTicker(sc.Interval)
	finalize := make(chan error, 2)
	gateway, err := net.Listen("tcp", fmt.Sprintf(":%d", sc.GatewayPort))
	if err != nil {
		finalize <- err
		return finalize
	}
	ctxCancel, cancel := context.WithCancel(ctx)
	receiver := make(chan []byte, 1)
	connected := make([]*socket.SignedConnection, 0)

	// gateway connections listener
	go func() {
		for {
			conn, err := gateway.Accept()
			if err != nil {
				gateway.Close()
				return
			}
			signed, err := socket.PromoteConnection(conn, credentials, socket.AcceptAllConnections)
			if err != nil {
				continue
			}
			go func() {
				for {
					data, err := signed.Read()
					if err != nil {
						return
					}
					receiver <- data
				}
			}()
		}
	}()

	go func() {
		validator := sc.State.Validator()
		actions := make([][]byte, 0)
		var err error
		for {
			select {
			case <-ticker.C:
				sc.Epoch += 1
				mutations := validator.Mutations()
				sc.State.Incorporate(mutations)
				block := &SimpleBlock{
					Epoch:   sc.Epoch,
					Actions: actions,
				}
				err = sc.Writer.WriteBlock(block)
				if err != nil {
					cancel()
				} else {
					sc.Recent = append(sc.Recent, actions)
					sc.Epoch += 1
					actions = actions[:0]
				}
			case data := <-receiver:
				if len(data) == 0 || !sc.State.Validator().Validate(data) {
					continue
				}
				if logger != nil {
					fmt.Println(logger(data))
				}
				actions = append(actions, data)
			case <-ctxCancel.Done():
				for _, c := range connected {
					c.Shutdown()
				}
				gateway.Close()
				sc.Writer.writer.Close()
				sc.State.Shutdown()
				ticker.Stop()
				finalize <- err
				return
			}
		}
	}()
	return finalize
}
