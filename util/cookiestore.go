package util

import (
	"encoding/hex"
	"io"
	"log"
	"os"

	"github.com/freehandle/breeze/crypto"
)

const cookieSessionDuration = 30 * 24 * 60 * 60 // epochs

type CookieStore struct {
	file       *os.File
	session    map[string]crypto.Token // cookie to token
	sessionend map[uint64][]string     //epoch to cookies
	position   map[crypto.Token]int64  // cookie to position on file
}

func (c *CookieStore) Close() {
	c.file.Close()
}

func (c *CookieStore) Unset(token crypto.Token, cookie string) {
	position, ok := c.position[token]
	if ok {
		bytes := make([]byte, crypto.Size)
		c.file.Seek(position+crypto.Size, 0)
		if n, err := c.file.Write(bytes); n != len(bytes) {
			log.Printf("unexpected error in cookie store: %v", err)
		}
	}
	delete(c.session, cookie)
}

func (c *CookieStore) Clean(epoch uint64) {
	cookies := c.sessionend[epoch]
	for _, cookie := range cookies {
		if token, ok := c.session[cookie]; ok {
			c.Unset(token, cookie)
		}
	}
}

func (c *CookieStore) Get(cookie string) (crypto.Token, bool) {
	token, ok := c.session[cookie]
	return token, ok
}

func (c *CookieStore) Set(token crypto.Token, cookie string, epoch uint64) bool {
	bytes, err := hex.DecodeString(cookie)
	if err != nil || len(bytes) != crypto.Size {
		return false
	}
	c.session[cookie] = token
	epochEnd := epoch + cookieSessionDuration
	c.sessionend[epochEnd] = append(c.sessionend[epochEnd], cookie)
	position, ok := c.position[token]
	if ok {
		c.file.Seek(position, 0)
	} else {
		bytes = append(token[:], bytes...) // token + cookie
		c.file.Seek(0, 2)
	}
	if n, err := c.file.Write(bytes); n != len(bytes) {
		log.Printf("unexpected error in cookie store: %v", err)
		return false
	}
	return true
}

func OpenCokieStore(path string, epoch uint64) *CookieStore {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Fatalf("could not open cookie store file: %v", err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("could not read cookie store file: %v", err)
	}
	if len(data)%(2*crypto.Size) != 0 {
		log.Fatalf("length of cookie store file incompatible: %v", len(data))
	}
	position := 0
	store := &CookieStore{
		file:       file,
		session:    make(map[string]crypto.Token),
		sessionend: make(map[uint64][]string),
		position:   make(map[crypto.Token]int64),
	}
	for n := 0; n < len(data)/(2*crypto.Size); n++ {
		var token crypto.Token
		copy(token[:], data[2*n*crypto.Size:(2*n+1)*crypto.Size])
		var hash crypto.Hash
		copy(hash[:], data[(2*n+1)*crypto.Size:2*(n+1)*crypto.Size])
		if !hash.Equal(crypto.ZeroHash) && !token.Equal(crypto.ZeroToken) {
			cookie := hex.EncodeToString(hash[:])
			store.session[cookie] = token
			store.position[token] = int64(position)
			endEpoch := epoch + cookieSessionDuration
			store.sessionend[endEpoch] = append(store.sessionend[endEpoch], cookie)
			store.position[token] = int64(2 * n * crypto.Size)
		}
	}
	return store
}
