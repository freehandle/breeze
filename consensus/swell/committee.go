package swell

import (
	"sort"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type Validator struct {
	Address string
	Token   crypto.Token
	Weight  int
}

type Validators []*Validator

type TokenHash struct {
	Token crypto.Token
	Hash  crypto.Hash
}

type TokenHashArray []TokenHash

func (h TokenHashArray) Len() int {
	return len(h)
}

func (h TokenHashArray) Less(i, j int) bool {
	for n := 0; n < crypto.Size; n++ {
		if h[i].Hash[n] < h[j].Hash[n] {
			return true
		}
	}
	return false
}

func (h TokenHashArray) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

type ChecksumWindowValidatorPool struct {
	hostname    string
	credentials crypto.PrivateKey
	order       []crypto.Token
	weights     map[crypto.Token]int
	consensus   []*socket.ChannelConnection
	blocks      *socket.PercolationPool
	validators  []socket.CommitteeMember
}

func SingleCommittee(credentials crypto.PrivateKey, hostname string) *ChecksumWindowValidatorPool {
	return &ChecksumWindowValidatorPool{
		hostname:    hostname,
		credentials: credentials,
		order:       []crypto.Token{credentials.PublicKey()},
		weights:     map[crypto.Token]int{credentials.PublicKey(): 1},
		consensus:   make([]*socket.ChannelConnection, 0),
		blocks:      socket.AssembleOwnPercolationPool(),
		validators:  []socket.CommitteeMember{{Token: credentials.PublicKey()}},
	}
}

func BroadcastPercolationRule(nodecount int) socket.PercolationRule {
	return func(epoch uint64) []int {
		nodes := make([]int, 0)
		for i := 0; i < nodecount; i++ {
			nodes = append(nodes, i)
		}
		return nodes
	}
}

func LaunchValidatorPool(validators Validators, credentials crypto.PrivateKey, seed []byte) *ChecksumWindowValidatorPool {
	pool := &ChecksumWindowValidatorPool{
		credentials: credentials,
	}
	return pool.PrepareNext(validators, seed)
}

func (v *ChecksumWindowValidatorPool) PrepareNext(validators Validators, seed []byte) *ChecksumWindowValidatorPool {

	pool := &ChecksumWindowValidatorPool{
		credentials: v.credentials,
		order:       make([]crypto.Token, 0),
		weights:     make(map[crypto.Token]int),
	}

	token := v.credentials.PublicKey()

	peers := make([]socket.CommitteeMember, 0)

	hashes := make(TokenHashArray, 0)
	for _, validator := range validators {
		pool.weights[validator.Token] = validator.Weight
		for w := 1; w <= validator.Weight; w++ {
			tokenhash := TokenHash{
				Token: validator.Token,
				Hash:  crypto.Hasher(append(append([]byte{byte(w)}, seed...), validator.Token[:]...)),
			}
			hashes = append(hashes, tokenhash)
		}
	}
	sort.Sort(hashes)
	for _, tokenhash := range hashes {
		pool.order = append(pool.order, tokenhash.Token)
	}

	for _, validator := range validators {
		if !validator.Token.Equal(token) {
			peers = append(peers, socket.CommitteeMember{
				Address: validator.Address,
				Token:   validator.Token,
			})
		}
	}

	pool.consensus = socket.AssembleChannelNetwork(peers, v.credentials, 5401, pool.hostname, v.consensus)
	pool.blocks = socket.AssemblePercolationPool(peers, v.credentials, 5400, pool.hostname, BroadcastPercolationRule(len(peers)), v.blocks)
	return pool
}