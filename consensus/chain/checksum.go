package chain

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

func (c *Blockchain) MarkCheckpoint(done chan bool) {
	c.mu.Lock()
	c.Cloning = true
	go func() {
		epoch := c.LastCommitEpoch
		hash := c.LastCommitHash
		clonedState := c.CommitState.Clone()
		if clonedState == nil {
			done <- false
			return
		}
		c.Checksum = &Checksum{
			Epoch:         epoch,
			State:         clonedState,
			LastBlockHash: hash,
			Hash:          clonedState.ChecksumHash(),
		}
		c.Cloning = false
		done <- true
	}()
	c.mu.Unlock()
}

type ChecksumStatement struct {
	Epoch     uint64
	Node      crypto.Token
	Address   string
	Naked     bool
	Hash      crypto.Hash
	Signature crypto.Signature
}

func PutChecksumStatement(d *ChecksumStatement, bytes *[]byte) {
	util.PutUint64(d.Epoch, bytes)
	util.PutToken(d.Node, bytes)
	util.PutString(d.Address, bytes)
	util.PutBool(d.Naked, bytes)
	util.PutHash(d.Hash, bytes)
	util.PutSignature(d.Signature, bytes)
}

func (d *ChecksumStatement) Serialize() []byte {
	bytes := make([]byte, 0)
	PutChecksumStatement(d, &bytes)
	return bytes
}

func ParseChecksumStatement(data []byte) *ChecksumStatement {
	parsed, _ := ParseChecksumStatementPosition(data, 0)
	return parsed
}

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
func (dressed *ChecksumStatement) IsDressed(naked *ChecksumStatement) bool {
	return naked.Naked && !(dressed.Naked) && naked.Epoch == dressed.Epoch && crypto.Hasher(append(naked.Node[:], naked.Hash[:]...)).Equal(dressed.Hash)
}
