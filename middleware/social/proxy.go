package social

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/util"
)

type LocalBlockChain[M Merger[M], B Blocker[M]] struct {
	Interval  time.Duration
	Receiver  chan []byte
	Listeners []chan []byte
	IO        io.WriteCloser
	State     Stateful[M, B]
	Epoch     uint64
}

// Very simple chain implementation for local storage
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

func (local *LocalBlockChain[M, B]) LoadState(genesis Stateful[M, B], source io.Reader, listeners []chan []byte) error {
	local.State = genesis
	validator := genesis.Validator()
	data := make([]byte, 0, 1<<20)
	n := 0
	count := uint32(0)
	for {
		buffer := make([]byte, (1<<20)-n)
		nbytes, err := source.Read(buffer)
		if err != nil && err != io.EOF {
			return err
		}
		if n > len(data)+nbytes {
			return fmt.Errorf("buffer overflow")
		}
		if n == len(data)+nbytes {
			local.State.Incorporate(validator.Mutations())
			return nil
		}
		data = append(data[n:], buffer[:nbytes]...)
		n = 0
		local.Epoch, n = util.ParseUint64(data, 0)
		// fmt.Printf("Loading epoch %d\n", local.Epoch)
		count, n = util.ParseUint32(data, n)
		actions := make([][]byte, count)
		// fmt.Printf("Loading %v actions %v\n", count, n)
		for i := 0; i < int(count); i++ {
			actions[i], n = util.ParseLargeByteArray(data, n)
			ok := validator.Validate(actions[i][1:])
			if !ok {
				// return fmt.Errorf("invalid action at position %d of block %d", i, local.Epoch)
				fmt.Printf("invalid action at position %d of block %d", i, local.Epoch)
			}
			if len(listeners) > 0 {
				for _, listener := range listeners {
					listener <- actions[i]
				}
			}
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
				bytes := []byte{0}
				util.PutUint64(local.Epoch, &bytes)
				for _, listener := range local.Listeners {
					listener <- bytes
				}
			case msg := <-local.Receiver:
				if len(msg) == 0 || msg[0] != messages.MsgAction {
					continue
				}
				if validator.Validate(msg[1:]) {
					copied := make([]byte, len(msg))
					copy(copied, msg)
					actions = append(actions, copied)
					for _, listener := range local.Listeners {
						listener <- msg
					}
				}
			}
		}
	}()
	return finalize
}
