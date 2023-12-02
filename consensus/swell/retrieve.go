package swell

import (
	"sync"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type retrievalStatus struct {
	mu     sync.Mutex
	done   bool
	output chan *chain.SealedBlock
}

func (r *retrievalStatus) Done(sealed *chain.SealedBlock) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return true
	}
	if sealed != nil {
		r.done = true
		r.output <- sealed
		return true
	}
	return false
}

func RetrieveBlock(epoch uint64, hash crypto.Hash, order []*socket.BufferedChannel) chan *chain.SealedBlock {
	output := make(chan *chain.SealedBlock)
	ellapse := 400 * time.Millisecond
	msg := chain.RequestBlockMessage(epoch, hash)
	status := retrievalStatus{
		mu:     sync.Mutex{},
		output: output,
	}
	for n, channel := range order {
		go func(n int, channel *socket.BufferedChannel, status *retrievalStatus) {
			time.Sleep(time.Duration(n) * ellapse)
			channel.SendSide(msg)
			data := channel.ReadSide()
			if len(data) == 0 {
				return
			}
			sealed := chain.ParseSealedBlock(data)
			if sealed != nil && sealed.Header.Epoch == epoch && sealed.Seal.Hash.Equal(hash) {
				status.Done(sealed)
				return
			}
		}(n, channel, &status)
	}
	return output
}
