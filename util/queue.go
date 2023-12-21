package util

import (
	"context"

	"github.com/freehandle/breeze/crypto"
)

type DataQueue[T any] struct {
	live  bool
	next  chan struct{}
	read  chan T
	write chan T
}

func (b *DataQueue[T]) Close() {
	b.live = false
	close(b.write)
}

func (b *DataQueue[T]) Pop() T {
	var data T
	if !b.live {
		return data
	}
	b.next <- struct{}{}
	data = <-b.read
	return data
}

func (b *DataQueue[T]) Push(data T) {
	if b.live {
		b.write <- data
	}
}

func NewDataQueue[T any](ctx context.Context, hash func(T) crypto.Hash) *DataQueue[T] {
	hashes := make(map[crypto.Hash]struct{})
	dataQueue := &DataQueue[T]{
		live:  true,
		next:  make(chan struct{}),
		read:  make(chan T),
		write: make(chan T),
	}
	go func() {
		defer func() {
			dataQueue.live = false
			close(dataQueue.read)
			close(dataQueue.next)
		}()
		buffer := make([]T, 0)
		waiting := false
		done := ctx.Done()
		for {
			select {
			case <-done:
				close(dataQueue.write)
			case data, ok := <-dataQueue.write:
				if !ok {
					return
				}
				if _, ok := hashes[hash(data)]; ok {
					continue
				}
				if waiting {
					dataQueue.read <- data
					waiting = false
				} else {
					buffer = append(buffer, data)
				}
			case <-dataQueue.next:
				if len(buffer) == 0 {
					waiting = true
				}
			}
		}
	}()
	return dataQueue
}
