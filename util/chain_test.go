package util

import (
	"context"
	"testing"
)

func TestChain(t *testing.T) {
	chain := NewChain[int](context.Background(), 1)

	finish := make(chan struct{})
	go func() {
		last := 0
		for {
			n := chain.Pop()
			if n != last+1 {
				t.Errorf("Wrong chain order")
			}
			last = n
			if n == 5 {
				finish <- struct{}{}
				return
			}
		}
	}()

	chain.Push(2, 2)
	chain.Push(4, 4)
	chain.Push(1, 1)
	chain.Push(5, 5)
	chain.Push(3, 3)
	<-finish
}
