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

type Validator interface {
	Validate([]byte) bool
}

type Stateful[T Merger[T], B Blocker[T]] interface {
	Validator(...T) B
	Incorporate(T)
	Shutdown()
	Checksum() crypto.Hash
	Clone() chan Stateful[T, B]
	Serialize() []byte
}

type StateFromBytes[T Merger[T], B Blocker[T]] func([]byte) (Stateful[T, B], bool)
