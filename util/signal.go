package util

import (
	"context"
)

type Await struct {
	live  bool
	next  chan struct{}
	read  chan struct{}
	write chan struct{}
}

func (b *Await) Close() {
	b.live = false
	close(b.write)
}

func (b *Await) Wait() bool {
	if !b.live {
		close(b.read)
		close(b.next)
		return false
	}
	b.next <- struct{}{}
	<-b.read
	return true
}

func (b *Await) Call() bool {
	if b.live {
		b.write <- struct{}{}
		return true
	}
	return false
}

func NewAwait(ctx context.Context) *Await {
	signal := &Await{
		live:  true,
		next:  make(chan struct{}),
		read:  make(chan struct{}),
		write: make(chan struct{}),
	}
	go func() {
		defer func() {
			signal.live = false
			close(signal.read)
			close(signal.next)
		}()
		hasWrite := 0
		waitingRead := 0
		done := ctx.Done()
		for {
			select {
			case <-done:
				close(signal.write)
			case _, ok := <-signal.write:
				if !ok {
					return
				}
				if waitingRead > 0 {
					signal.read <- struct{}{}
					waitingRead -= 1
				} else {
					hasWrite += 1
				}
			case <-signal.next:
				if hasWrite == 0 {
					waitingRead += 1
				} else {
					signal.read <- struct{}{}
					hasWrite -= 1
				}
			}
		}
	}()
	return signal
}
