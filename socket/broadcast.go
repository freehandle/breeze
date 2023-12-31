package socket

import (
	"context"
	"fmt"

	"github.com/freehandle/breeze/crypto"
)

// PercolationRule defines a rule for diffusion of block data among validators.
// At a given epoch a node will be required to broadcast data to a subsect of
// other nodes.
type PercolationRule func(epoch uint64) []int

// MergeRules combines several rules into a single one. If a node is designated
// by any of the rules for a given epoch, it will be included in the merged rule.
func MergeRules(r ...PercolationRule) PercolationRule {
	return func(epoch uint64) []int {
		nodes := make([]int, 0)
		for _, rule := range r {
			nodesRule := rule(epoch)
			for _, node := range nodesRule {
				isNew := true
				for _, existing := range nodes {
					if existing == node {
						isNew = false
						break
					}
				}
				if isNew {
					nodes = append(nodes, node)
				}
			}
		}
		return nodes
	}
}

// PercolationPool is a pool of BufferedChannel connections to other nodes in
// the peer group and a percolation rule that orients how any messgae is
// transmitted between nodes until every node is reached.
type PercolationPool struct {
	connections []*BufferedMultiChannel
	rule        PercolationRule
}

func (p *PercolationPool) GetLeader(token crypto.Token) (*BufferedMultiChannel, []*BufferedMultiChannel) {
	for n, connection := range p.connections {
		if connection.Conn.Token.Equal(token) {
			return connection, append(p.connections[:n], p.connections[n+1:]...)
		}
	}
	return nil, nil
}

// Send sends a message to all nodes designated in the percolation rule.
func (b *PercolationPool) Send(epoch uint64, data []byte) {
	nodes := b.rule(epoch)
	for _, node := range nodes {
		if b.connections[node] != nil {
			b.connections[node].Send(epoch, data)
		}
	}
}

// AssembleOwnPercolationPool creates an empty pool of connections. This is used
// for the case where the network is composed of a single node.
func AssembleOwnPercolationPool() *PercolationPool {
	return &PercolationPool{
		connections: make([]*BufferedMultiChannel, 0),
		rule:        func(epoch uint64) []int { return []int{} },
	}
}

// AssemblePercolationPool creates a pool of connections to other nodes in the
// peer group. It uses live connection over an existing pool if provided.
func AssemblePercolationPool(ctx context.Context, peers []TokenAddr, credentials crypto.PrivateKey, port int, hostname string, rule PercolationRule, existing *PercolationPool) *PercolationPool {
	token := credentials.PublicKey()
	pool := PercolationPool{
		connections: make([]*BufferedMultiChannel, len(peers)),
		rule:        rule,
	}
	members := make([]TokenAddr, 0)
	for _, peer := range peers {
		if !peer.Token.Equal(token) {
			members = append(members, TokenAddr{
				Addr:  fmt.Sprintf("%v:%v", peer.Addr, port),
				Token: peer.Token,
			})
		}
	}
	connected := make([]*BufferedMultiChannel, 0)
	if existing != nil {
		connected = existing.connections
	}
	committee := AssembleCommittee[*BufferedMultiChannel](ctx, members, connected, NewBufferredMultiChannel, credentials, port, hostname)
	connections := <-committee
	for n, member := range peers {
		if !member.Token.Equal(token) {
			for _, c := range connections {
				if c.Conn.Token.Equal(member.Token) {
					pool.connections[n] = c
					break
				}
			}
		}
	}
	return &pool
}
