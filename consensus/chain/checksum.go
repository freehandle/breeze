package chain

import (
	"fmt"
	"log/slog"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

// MarkCheckpoint marks the current state as a checkpoint. It creates a clone
// of the state at the last commit epoch and calculates the checksum of the
// state. It returns a true value on the provided channel if the clone
// operation was successful. Otherwise, it returns false.
func (c *Blockchain) MarkCheckpoint() {
	fmt.Println("tmj")
	c.mu.Lock()
	c.Cloning = true
	go func() {
		epoch := c.LastCommitEpoch
		hash := c.LastCommitHash
		clonedState := c.CommitState.Clone()
		if clonedState == nil {
			fmt.Println("cloned state is nil")
			return
		}
		c.NextChecksum = &Checksum{
			Epoch:         epoch,
			State:         clonedState,
			LastBlockHash: hash,
			Hash:          clonedState.ChecksumHash(),
		}
		c.Cloning = false
		slog.Info("MarkCheckPoint: job completed", "epoch", c.NextChecksum.Epoch, "hash", c.NextChecksum.Hash)
		fmt.Println(c.Checksum.Hash)
	}()
	c.mu.Unlock()

}

// ChecksumStatement is a message every candidate node for validating during
// the next checksum window period msut send to current validator nodes. It is
// first sent with the checksum hash hashed with the node's token and with
// Naked marked as false. It is aftwards sent with Naked marked as true and
// the naked checksum hash. Inconsistent ChecksumStatements for the same epoch
// are considered illegal by the swell protocol and might be penalised depending
// on the p´revailing perssion rules.
type ChecksumStatement struct {
	Epoch     uint64
	Node      crypto.Token
	Address   string
	Naked     bool
	Hash      crypto.Hash
	Signature crypto.Signature
}

func NewCheckSum(epoch uint64, node crypto.PrivateKey, address string, naked bool, hash crypto.Hash) *ChecksumStatement {
	statement := &ChecksumStatement{
		Epoch:   epoch,
		Node:    node.PublicKey(),
		Address: address,
		Naked:   naked,
		Hash:    hash,
	}
	bytes := make([]byte, 0)
	putChecksumStatementForSign(statement, &bytes)
	statement.Signature = node.Sign(bytes)
	return statement
}

// PutChecksumStatement serializes a ChecksumStatement to a byte slice and
// appends it to the provided byte slice.
func putChecksumStatementForSign(d *ChecksumStatement, bytes *[]byte) {
	util.PutUint64(d.Epoch, bytes)
	util.PutToken(d.Node, bytes)
	util.PutString(d.Address, bytes)
	util.PutBool(d.Naked, bytes)
	util.PutHash(d.Hash, bytes)
}

// PutChecksumStatement serializes a ChecksumStatement to a byte slice and
// appends it to the provided byte slice.
func PutChecksumStatement(d *ChecksumStatement, bytes *[]byte) {
	putChecksumStatementForSign(d, bytes)
	util.PutSignature(d.Signature, bytes)
}

// Serialize serializes a ChecksumStatement to a byte slice.
func (d *ChecksumStatement) Serialize() []byte {
	bytes := make([]byte, 0)
	PutChecksumStatement(d, &bytes)
	return bytes
}

// ParseChecksumStatement parses a ChecksumStatement from a byte slice.
func ParseChecksumStatement(data []byte) *ChecksumStatement {
	parsed, _ := ParseChecksumStatementPosition(data, 0)
	return parsed
}

// ParseChecksumStatementPosition parses a ChecksumStatement in the middle of
// a byte slice and returns the parsed ChecksumStatement and the position
func ParseChecksumStatementPosition(data []byte, position int) (*ChecksumStatement, int) {
	dressed := ChecksumStatement{}
	dressed.Epoch, position = util.ParseUint64(data, position)
	dressed.Node, position = util.ParseToken(data, position)
	dressed.Address, position = util.ParseString(data, position)
	dressed.Naked, position = util.ParseBool(data, position)
	dressed.Hash, position = util.ParseHash(data, position)
	dressed.Signature, _ = util.ParseSignature(data, position)
	if dressed.Node.Verify(data[0:position], dressed.Signature) {
		return &dressed, position
	}
	return nil, len(data)
}

// IsDressed returns true if the naked ChecksumStatement is compatible with the
// dressed ChecksumStatement. It returns false otherwise.
func (dressed *ChecksumStatement) IsDressed(naked *ChecksumStatement) bool {
	return naked.Naked && !(dressed.Naked) && naked.Epoch == dressed.Epoch && crypto.Hasher(append(naked.Node[:], naked.Hash[:]...)).Equal(dressed.Hash)
}
