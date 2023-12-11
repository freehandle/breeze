package swell

import (
	"sync"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

// RetriveBlock is called by a validator that is not in posession of a block
// compatible with the bft consensus hash. It will try to retrive such block
// from other nodes in the sequence given by the order parameter. It returns a
// channel to a selaed block.
// TODO: it does nothing when the block is not found.
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
			if status.done {
				return
			}
			channel.SendSide(msg)
			data := channel.ReadSide()
			if len(data) == 0 {
				return
			}
			sealed := chain.ParseSealedBlock(data)
			if sealed != nil && sealed.Header.Epoch == epoch && sealed.Seal.Hash.Equal(hash) {
				status.Done(sealed)
			}
		}(n, channel, &status)
	}
	return output
}

// retrievalStatus keeps track of the retrieval process. if done is true all
// other non-initiated requests must be aborted and all the initiated requtest
// must be ignored.
type retrievalStatus struct {
	mu     sync.Mutex
	done   bool
	output chan *chain.SealedBlock
}

// Done is called when the block is found or when the retrieval process is
// completed.
func (r *retrievalStatus) Done(sealed *chain.SealedBlock) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	if sealed != nil {
		r.done = true
		r.output <- sealed
	}
}
