package util

import (
	"context"
	"fmt"
)

type orderedItem[T any] struct {
	data  T
	epoch uint64
}

// Chain provides a data structure that allows order data to be pushed possibly
// out of order and popped ordered.
type Chain[T any] struct {
	Epoch uint64
	live  bool
	next  chan struct{}
	read  chan T
	write chan orderedItem[T]
}

func (b *Chain[T]) Close() {
	b.live = false
	close(b.write)
}

func (b *Chain[T]) Pop() T {
	var data T
	if !b.live {
		close(b.next)
		close(b.read)
		return data
	}
	b.next <- struct{}{}
	data = <-b.read
	return data
}

func (b *Chain[T]) Push(data T, epoch uint64) {
	if b.live {
		b.write <- orderedItem[T]{data, epoch}
	}
}

func NewChain[T any](ctx context.Context, start uint64) *Chain[T] {
	chain := &Chain[T]{
		live:  true,
		Epoch: start,
		next:  make(chan struct{}),
		read:  make(chan T),
		write: make(chan orderedItem[T]),
	}
	go func() {
		defer func() {
			chain.live = false
			close(chain.write)
		}()
		buffer := make([]orderedItem[T], 0)
		waiting := false
		done := ctx.Done()
		for {
			select {
			case <-done:
				return
			case block, ok := <-chain.write:
				if !ok {
					return
				}
				if block.epoch < chain.Epoch {
					continue
				}
				// if block is for current epoch and there is a waiting read,
				// send it directly. Otherwise put the block in the right
				// spot in the buffer.
				fmt.Println("write", block.epoch, chain.Epoch, waiting)
				if block.epoch == chain.Epoch && waiting {
					chain.read <- block.data
					chain.Epoch += 1
					waiting = false
				} else {
					inserted := false
					for n, item := range buffer {
						if item.epoch > block.epoch {
							buffer = append(append(buffer[0:n], block), buffer[n:]...)
							inserted = true
							break
						}
					}
					if !inserted {
						buffer = append(buffer, block)
					}
				}
			case <-chain.next:
				if len(buffer) == 0 || buffer[0].epoch > chain.Epoch {
					waiting = true
				} else {
					chain.read <- buffer[0].data
					buffer = buffer[1:]
					chain.Epoch += 1
					waiting = false
				}
			}
		}
	}()
	return chain
}
