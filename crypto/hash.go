package crypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
)

type Hash [Size]byte

var hashLength = base64.StdEncoding.EncodedLen(Size)

func (h Hash) MarshalText() (text []byte, err error) {
	text = make([]byte, hashLength)
	base64.StdEncoding.Encode(text, h[:])
	return
}

func DecodeHash(text string) Hash {
	var hash Hash
	base64.StdEncoding.Decode(hash[:], []byte(text))
	return hash
}

func EncodeHash(h Hash) string {
	text := make([]byte, hashLength)
	base64.StdEncoding.Encode(text, h[:])
	return string(text)
}

func (h Hash) UnmarshalText(text []byte) error {
	_, err := base64.StdEncoding.Decode(h[:], text)
	return err
}

var ZeroHash Hash = Hasher([]byte{})
var ZeroValueHash Hash

func (hash Hash) ToInt64() int64 {
	return int64(hash[0]) + (int64(hash[1]) << 8) + (int64(hash[2]) << 16) + (int64(hash[3]) << 24)
}

func BytesToHash(bytes []byte) Hash {
	var hash Hash
	if len(bytes) != Size {
		return hash
	}
	copy(hash[:], bytes)
	return hash
}

func (h Hash) Equal(another Hash) bool {
	return h == another
}

func (h Hash) Equals(another []byte) bool {
	if len(another) < Size {
		return false
	}
	return bytes.Equal(h[:], another[:Size])
}

func Hasher(data []byte) Hash {
	return Hash(sha256.Sum256(data))
}

func HashToken(token Token) Hash {
	return Hash(sha256.Sum256(token[:]))
}
