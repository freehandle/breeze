/*
Permissions implement a permission interface to determine if a node is
allowed to participate in consensus and punish nodes that violate the
consensus rules.

The ProofOfStake permission implementation requires a minimum amount of tokens
staged on a deposit contract to be allowed to participate in consensus. The
balance deposited is checked against the state of the chain at the time of
checksum window creation. The ProofOfStake implementation punishes nodes that
violate the consensus rules by slashing their deposit.

The ProofOfAuthority permission implementation requires a list of authorized
tokens to be allowed to participate in consensus. The ProofOfAuthority does not
contemplate punishment other than be automatically removed from the list of
authorized tokens.
*/
package permission

import (
	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
)

type Permissionless struct{}

func (p Permissionless) Punish(duplicates *bft.Duplicate, weights map[crypto.Token]int) map[crypto.Token]uint64 {
	return nil
}

func (p Permissionless) DeterminePool(chain *chain.Blockchain, candidates []crypto.Token) map[crypto.Token]int {
	validated := make(map[crypto.Token]int)
	for _, token := range candidates {
		validated[token] = 1
	}
	return validated
}
