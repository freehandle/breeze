package swell

import (
	"context"
	"sort"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

type Validator struct {
	Address string
	Token   crypto.Token
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

type Committee struct {
	ctx         context.Context
	cancel      context.CancelFunc
	hostname    string
	credentials crypto.PrivateKey
	order       []crypto.Token
	weights     map[crypto.Token]int
	consensus   []*socket.ChannelConnection
	blocks      *socket.PercolationPool
	validators  []socket.CommitteeMember
}

func (c *Committee) Serialize() []byte {
	bytes := []byte{messages.MsgNetworkTopologyResponse}
	util.PutUint16(uint16(len(c.order)), &bytes)
	for n := 0; n < len(c.order); n++ {
		util.PutToken(c.order[n], &bytes)
	}
	util.PutUint16(uint16(len(c.validators)), &bytes)
	for n := 0; n < len(c.validators); n++ {
		util.PutToken(c.validators[n].Token, &bytes)
		util.PutString(c.validators[n].Address, &bytes)
	}
	return bytes
}

func ParseCommitee(bytes []byte) ([]crypto.Token, []socket.CommitteeMember) {
	if len(bytes) < 1 || bytes[0] != messages.MsgNetworkTopologyResponse {
		return nil, nil
	}
	order := make([]crypto.Token, 0)
	validators := make([]socket.CommitteeMember, 0)
	var orderCount, validatorCount uint16
	position := 1
	orderCount, position = util.ParseUint16(bytes, 1)
	for n := uint16(0); n < orderCount; n++ {
		var token crypto.Token
		token, position = util.ParseToken(bytes, position)
		order = append(order, token)
	}
	if position >= len(bytes) {
		return nil, nil
	}
	validatorCount, position = util.ParseUint16(bytes, position)
	for n := uint16(0); n < validatorCount; n++ {
		member := socket.CommitteeMember{}
		member.Token, position = util.ParseToken(bytes, position)
		member.Address, position = util.ParseString(bytes, position)
		validators = append(validators, member)
	}
	if position != len(bytes) {
		return nil, nil
	}
	return order, validators
}

func SingleCommittee(credentials crypto.PrivateKey, hostname string) *Committee {
	ctx, cancel := context.WithCancel(context.Background())
	return &Committee{
		ctx:         ctx,
		cancel:      cancel,
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

func sortCandidates(candidates map[crypto.Token]int, seed []byte, committeeSize int) []crypto.Token {
	hashes := make(TokenHashArray, 0)
	for token, weight := range candidates {
		for w := 1; w <= weight; w++ {
			tokenhash := TokenHash{
				Token: token,
				Hash:  crypto.Hasher(append(append([]byte{byte(w)}, token[:]...), seed...)),
			}
			hashes = append(hashes, tokenhash)
		}
	}
	sort.Sort(hashes)
	ordered := make([]crypto.Token, committeeSize)
	for count := 0; count < committeeSize; count++ {
		ordered[count] = hashes[count%len(hashes)].Token
	}
	return ordered
}

func LaunchValidatorPool(ctx context.Context, validators Validators, credentials crypto.PrivateKey, hostname string) *Committee {
	ctx, cancel := context.WithCancel(ctx)
	pool := &Committee{
		ctx:         ctx,
		cancel:      cancel,
		hostname:    hostname,
		credentials: credentials,
	}
	return pool.PrepareNext(validators)
}

func (v *Committee) PrepareNext(validators Validators) *Committee {
	ctx, cancel := context.WithCancel(v.ctx)
	pool := &Committee{
		ctx:         ctx,
		cancel:      cancel,
		hostname:    v.hostname,
		credentials: v.credentials,
		order:       make([]crypto.Token, 0),
		weights:     make(map[crypto.Token]int),
		validators:  make([]socket.CommitteeMember, 0),
	}

	token := v.credentials.PublicKey()
	peers := make([]socket.CommitteeMember, 0)

	for _, validator := range validators {
		if weight, ok := pool.weights[validator.Token]; ok {
			pool.weights[validator.Token] = weight + 1
		} else {
			pool.weights[validator.Token] = 1
			member := socket.CommitteeMember{
				Address: validator.Address,
				Token:   validator.Token,
			}
			pool.validators = append(pool.validators, member)
			if !validator.Token.Equal(token) {
				peers = append(peers, socket.CommitteeMember{
					Address: validator.Address,
					Token:   validator.Token,
				})
			}
		}
		pool.order = append(pool.order, validator.Token)
	}
	pool.consensus = socket.AssembleChannelNetwork(ctx, peers, v.credentials, 5401, pool.hostname, v.consensus)
	pool.blocks = socket.AssemblePercolationPool(ctx, peers, v.credentials, 5400, pool.hostname, BroadcastPercolationRule(len(peers)), v.blocks)
	return pool
}
