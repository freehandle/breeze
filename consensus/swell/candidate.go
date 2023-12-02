package swell

import (
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
)

const CandidateMsg byte = 200

// getConsensus tries to fiend a 2/3 + 1 consensus over a hash among members
func getConsensusHash(naked map[crypto.Token]*chain.ChecksumStatement, members map[crypto.Token]int) (crypto.Hash, bool) {
	totalweight := 0
	for _, weight := range members {
		totalweight += weight
	}
	weightPerHash := make(map[crypto.Hash]int)
	for _, statement := range naked {
		weight := weightPerHash[statement.Hash] + members[statement.Node]
		weightPerHash[statement.Hash] = weight
		if weight > 2*totalweight/3 {
			return statement.Hash, true
		}
	}
	return crypto.ZeroHash, false
}

// GetConsensusTokens checks if there is a naked hash with weight greater than
// 2/3 among current members of the validator pool. If so, Sit returns the tokens
// of all candidates that have provided naked and dressed statements for that
// hash.
func GetConsensusTokens(naked, dressed map[crypto.Token]*chain.ChecksumStatement, members map[crypto.Token]int, epoch uint64) []crypto.Token {
	hash, ok := getConsensusHash(naked, members)
	if !ok {
		return nil
	}
	tokens := make([]crypto.Token, 0)
	for _, nake := range naked {
		if nake.Hash.Equal(hash) {
			if dress, ok := dressed[nake.Node]; ok {
				if dress.IsDressed(nake) {
					tokens = append(tokens, nake.Node)
				}
			}
		}
	}
	return tokens
}
