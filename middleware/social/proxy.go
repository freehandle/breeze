package social

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

type LocalChainConfig[M Merger[M], B Blocker[M]] struct {
	Credentials  crypto.PrivateKey
	ProtocolCode uint32
	Interval     time.Duration
	Listeners    []chan []byte
	Genesis      Stateful[M, B]
}

type LocalBlockChain[M Merger[M], B Blocker[M]] struct {
	Credentials  crypto.PrivateKey
	ProtocolCode uint32
	Interval     time.Duration
	Receiver     chan []byte
	Listeners    []chan []byte
	IO           io.WriteCloser
	State        Stateful[M, B]
	Epoch        uint64
}

func (local *LocalBlockChain[M, B]) PeristActions(actions [][]byte) error {
	bytes := make([]byte, 0)
	util.PutUint64(local.Epoch, &bytes)
	util.PutUint32(uint32(len(actions)), &bytes)
	for _, action := range actions {
		util.PutLargeByteArray(action, &bytes)
	}
	_, err := local.IO.Write(bytes)
	if err != nil {
		return err
	}
	return nil
}

func OpenChain[M Merger[M], B Blocker[M]](IO io.ReadWriteCloser, cfg *LocalChainConfig[M, B]) (*LocalBlockChain[M, B], error) {
	state, epoch, err := StateFromGenesis(cfg.Genesis, IO, cfg.Listeners)
	if err != nil {
		return nil, err
	}
	local := &LocalBlockChain[M, B]{
		Credentials:  cfg.Credentials,
		ProtocolCode: cfg.ProtocolCode,
		Interval:     cfg.Interval,
		Receiver:     make(chan []byte),
		Listeners:    cfg.Listeners,
		IO:           IO,
		State:        state,
		Epoch:        epoch,
	}
	return local, nil
}

func StateFromGenesis[T Merger[T], B Blocker[T]](genesis Stateful[T, B], source io.Reader, listeners []chan []byte) (Stateful[T, B], uint64, error) {
	validator := genesis.Validator()
	buffer := make([]byte, 1<<20)
	n := 0
	for {
		remaning := make([]byte, 1<<20-n)
		nbytes, err := source.Read(remaning)
		if err != nil && err != io.EOF {
			return nil, 0, err
		}
		data := append(buffer[n:], remaning[:nbytes]...)
		n := 0
		epoch, n := util.ParseUint64(data, 0)
		count, n := util.ParseUint32(data, n)
		actions := make([][]byte, count)
		for i := 0; i < int(count); i++ {
			actions[i], n = util.ParseLargeByteArray(data, n)
			ok := validator.Validate(actions[i])
			if !ok {
				return nil, 0, fmt.Errorf("invalid action at position %d of block %d", i, epoch)
			}
			if len(listeners) > 0 {
				for _, listener := range listeners {
					listener <- actions[i]
				}
			}
		}
		if n > len(data) {
			return nil, 0, fmt.Errorf("invalid block at epoch %d", epoch)
		}
		genesis.Incorporate(validator.Mutations())
		if err == io.EOF {
			return genesis, epoch, nil
		}
	}
}

func (local *LocalBlockChain[M, B]) Start(ctx context.Context) chan error {
	finalize := make(chan error, 2)
	ticker := time.NewTicker(local.Interval)
	validator := local.State.Validator()
	go func() {
		actions := make([][]byte, 0)
		for {
			select {
			case <-ctx.Done():
				local.IO.Close()
				local.State.Shutdown()
				ticker.Stop()
				finalize <- nil
				return
			case <-ticker.C:
				local.State.Incorporate(validator.Mutations())
				validator = local.State.Validator()
				local.PeristActions(actions)
				actions = make([][]byte, 0)
				local.Epoch += 1
			case msg := <-local.Receiver:
				if validator.Validate(msg) {
					actions = append(actions, msg)
					for _, listener := range local.Listeners {
						listener <- msg
					}
				}
			}
		}
	}()
	return finalize
}