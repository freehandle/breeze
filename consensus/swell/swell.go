package swell

import (
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
)

// ChecksumWindowValidators defines the order of tokens for the current checksum
// window and the weight of each token.
type ChecksumWindowValidators struct {
	order   []crypto.Token
	weights map[crypto.Token]int
}

// SwellNode is the basic structure to run a swell node either as a validator,
// as a candidate validator or simplu as an iddle observer.
// the internal clock of the node is used to determine the current epoch of the
// breeze network.
// It can be optionally linked to a relay node to communicate with the ouside
// world. Relay network is used to gather proposed actions through the actions
// gateway and to send block events to other interested parties. Honest validator
// must keep the relay network open with and adequate firewall rule. Swell
// protocol does not dictate rules for the realy network nonetheless.
type SwellNode struct {
	clockEpoch  uint64                    // current epoch according to node internal clock
	validators  *ChecksumWindowValidators // valdiators for the current cheksum window
	credentials crypto.PrivateKey         // credentials for the node
	blockchain  *chain.Blockchain         // node's version of breeze blockchain
	actions     *store.ActionStore        // actions received through the actions gateway
	config      SwellNetworkConfiguration // parameters of the underlying network
	active      chan chan error
	relay       *relay.Node // (optional) relay network
	hostname    string      // "localhost" or empty for internet, anything else for testing
}

// AddSealedBlock incorporates a sealed block into the node's blockchain.
func (s *SwellNode) AddSealedBlock(sealed *chain.SealedBlock) {
	s.blockchain.AddSealedBlock(sealed)
}

func (s *SwellNode) PurgeActions(actions *chain.ActionArray) {
	for n := 0; n < actions.Len(); n++ {
		hash := crypto.Hasher(actions.Get(n))
		s.actions.Exlude(hash)
	}
}

// Permanently exlude action from the underluing action store.
func (s *SwellNode) PurgeAction(hash crypto.Hash) {
	s.actions.Exlude(hash)
}
