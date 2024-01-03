package social

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

type ProtocolBlock struct {
	Epoch       uint64
	Actions     [][]byte
	Invalidated []crypto.Hash
	Hash        crypto.Hash
	Publisher   crypto.Token
	Siganture   crypto.Signature
}

type ProtocolBuilder struct {
	data   []byte
	status byte
}

func (p *ProtocolBuilder) Building() bool {
	return p.status == 0
}

func (p *ProtocolBuilder) Sealed() bool {
	return p.status == 1
}

func (p *ProtocolBuilder) Commit() bool {
	return p.status == 2
}

func NewProtocolBuilder(epoch uint64) *ProtocolBuilder {
	data := make([]byte, 0)
	util.PutUint64(epoch, &data)
	return &ProtocolBuilder{
		data: data,
	}
}

func (p *ProtocolBuilder) AddAction(action []byte) {
	util.PutByteArray(action, &p.data)
}

func (p *ProtocolBuilder) Seal() crypto.Hash {
	p.status = 1
	zero := make([]byte, 0)
	util.PutByteArray(zero, &p.data)
	hash := crypto.Hasher(p.data)
	return hash
}

func (p *ProtocolBuilder) Finalize(invalidate []crypto.Hash, publisher crypto.PrivateKey) {
	util.PutHashArray(invalidate, &p.data)
	hash := crypto.Hasher(p.data)
	util.PutHash(hash, &p.data)
	util.PutToken(publisher.PublicKey(), &p.data)
	signature := publisher.Sign(hash[:])
	util.PutSignature(signature, &p.data)
	p.status = 2
}

func (p *ProtocolBuilder) Bytes() []byte {
	return p.data
}

func (p *ProtocolBlock) Serialize() []byte {
	bytes := make([]byte, 0)
	util.PutUint64(p.Epoch, &bytes)
	for _, action := range p.Actions {
		util.PutByteArray(action, &bytes)
	}
	util.PutByteArray([]byte{}, &bytes)
	util.PutHashArray(p.Invalidated, &bytes)
	util.PutHash(p.Hash, &bytes)
	util.PutToken(p.Publisher, &bytes)
	util.PutSignature(p.Siganture, &bytes)
	return bytes
}

func ParseProtocolBlockWithPosition(data []byte, position int) (*ProtocolBlock, int) {
	if position >= len(data) {
		return nil, position
	}
	protocol := ProtocolBlock{Actions: make([][]byte, 0)}
	protocol.Epoch, position = util.ParseUint64(data, position)
	for {
		data, position = util.ParseByteArray(data, position)
		if len(data) == 0 {
			break
		}
		protocol.Actions = append(protocol.Actions, data)
	}
	protocol.Invalidated, position = util.ParseHashArray(data, position)
	hash := crypto.Hasher(data[:position])
	protocol.Hash, position = util.ParseHash(data, position)
	if !hash.Equal(protocol.Hash) {
		return nil, len(data) + 1
	}
	protocol.Publisher, position = util.ParseToken(data, position)
	protocol.Siganture, position = util.ParseSignature(data, position)
	if protocol.Publisher.Verify(hash[:], protocol.Siganture) {
		return nil, position
	}
	return &protocol, position

}

func ParseProtocolBlock(data []byte) *ProtocolBlock {
	protocol, _ := ParseProtocolBlockWithPosition(data, 0)
	return protocol
}
