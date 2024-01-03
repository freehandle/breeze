package social

import (
	"github.com/freehandle/breeze/crypto"
)

type Merger[T any] interface {
	Merge(...T) T
}

type Blocker[T Merger[T]] interface {
	Validate([]byte) bool
	Mutations() T
}

type Stateful[T Merger[T], B Blocker[T]] interface {
	Validator(...T) B
	Incorporate(T)
	Shutdown()
	ChecksumPoint() crypto.Hash
	Recover() error
}
