package messages

import (
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

// TODO: this must be revised
const (
	MsgBlock          byte = iota // breeze protocol new block with heder
	MsgAction                     // sinsgle action
	MsgActionArray                // multiple actions
	MsgSyncRequest                // Request Syncrhonization starting at given epoch
	MsgActionSubmit               // Submit action to the network
	MsgSyncError                  // Cannot synchronize
	MsgNewBlock                   // Breeze new block with header
	MsgSeal                       // Breeze Seal Block with seal
	MsgCommit                     // Breeze Node own Commit Block with invalidated
	MsgBuilding                   // Block under constructions
	MsgSealedBlock                // Block Seal
	MsgCommittedBlock             // Block Commit

	MsgProtocolHeader      // Sub-Protocol New Block Message
	MsgProtocolActionArray // Sub-Protocol Action Array Message
	MsgProtocolNewAction   // Sub-Protocol New Action Message
	MsgProtocolSeal        // Sub-Protool Seal Block Message
	MsgProtocolCommit      // Sub-Protocol Commit Block Message
	MsgProtocolSealedBlock
	MsgProtocolCommitBlock // Sub-Protocol Full Block Message
	MsgCommittee

	MsgRequestBlock // Request a block
	MsgClockSync
	MsgSyncChecksum
	MsgSyncStateWallets
	MsgSyncStateDeposits
	MsgSyncStateEpochAndHash
	MsgChecksumStatement
	MsgNetworkTopologyReq
	MsgNetworkTopologyResponse
	MsgNextCommittee
	MsgSubscribeBlockEvents

	MsgActionForward
	MsgActionSealed
	MsgActionCommit

	MsgError
)

type NetworkTopology struct {
	Start      uint64
	End        uint64
	StartAt    time.Time
	Order      []crypto.Token
	Validators []socket.TokenAddr
}

func (n *NetworkTopology) Serialize() []byte {
	bytes := []byte{MsgNetworkTopologyResponse}
	util.PutUint64(n.Start, &bytes)
	util.PutUint64(n.End, &bytes)
	util.PutTime(n.StartAt, &bytes)
	util.PutUint16(uint16(len(n.Order)), &bytes)
	for _, token := range n.Order {
		util.PutToken(token, &bytes)
	}
	util.PutUint16(uint16(len(n.Validators)), &bytes)
	for _, validator := range n.Validators {
		util.PutToken(validator.Token, &bytes)
		util.PutString(validator.Addr, &bytes)
	}
	return bytes
}

func ParseNetworkTopologyMessage(data []byte) *NetworkTopology {
	if len(data) < 1 || data[0] != MsgNetworkTopologyResponse {
		return nil
	}
	var count uint16
	position := 1
	topology := NetworkTopology{}
	topology.Start, position = util.ParseUint64(data, position)
	topology.End, position = util.ParseUint64(data, position)
	topology.StartAt, position = util.ParseTime(data, position)

	count, position = util.ParseUint16(data, position)
	topology.Order = make([]crypto.Token, count)
	for n := uint16(0); n < count; n++ {
		topology.Order[n], position = util.ParseToken(data, position)
	}
	count, position = util.ParseUint16(data, position)
	topology.Validators = make([]socket.TokenAddr, count)
	for n := uint16(0); n < count; n++ {
		member := socket.TokenAddr{}
		member.Token, position = util.ParseToken(data, position)
		member.Addr, position = util.ParseString(data, position)
	}
	if position != len(data) {
		return nil
	}
	return &topology
}

func BlockMessage(block []byte) []byte {
	return append([]byte{MsgBlock}, block...)
}

func RequestBlockMessage(epoch uint64, hash crypto.Hash) []byte {
	bytes := []byte{MsgRequestBlock}
	util.PutUint64(epoch, &bytes)
	util.PutHash(hash, &bytes)
	return bytes
}

func ActionMessage(action []byte) []byte {
	return append([]byte{MsgAction}, action...)
}

func SyncMessage(epoch uint64) []byte {
	data := []byte{MsgSyncRequest}
	util.PutUint64(epoch, &data)
	return data
}

func SubmitActionMessage(action []byte) []byte {
	return append([]byte{MsgActionSubmit}, action...)
}

func SyncErrroMessage(msg string) []byte {
	return append([]byte{MsgSyncError}, []byte(msg)...)
}

func NewBlockMessage(header []byte) []byte {
	return append([]byte{MsgNewBlock}, header...)
}

func BlockSealMessage(epoch uint64, seal []byte) []byte {
	bytes := []byte{MsgSeal}
	util.PutUint64(epoch, &bytes)
	return append(bytes, seal...)
}

func CommitBlock(epoch uint64, commit []byte) []byte {
	bytes := []byte{MsgCommit}
	util.PutUint64(epoch, &bytes)
	return append(bytes, commit...)
}

func Commit(epoch uint64, hash crypto.Hash, commit []byte) []byte {
	bytes := []byte{MsgCommit}
	util.PutUint64(epoch, &bytes)
	util.PutHash(hash, &bytes)
	return append(bytes, commit...)
}

func ParseEpochAndHash(data []byte) (uint64, crypto.Hash, []byte) {
	if len(data) < 1 {
		return 0, crypto.Hash{}, nil
	}
	position := 1
	epoch, position := util.ParseUint64(data, position)
	hash, position := util.ParseHash(data, position)
	if position < len(data) {
		return epoch, hash, data[position:]
	}
	return 0, crypto.Hash{}, nil
}

func SealedBlock(sealed []byte) []byte {
	return append([]byte{MsgSealedBlock}, sealed...)
}

func SealedAction(action crypto.Hash, block uint64, blockHash crypto.Hash) []byte {
	msg := []byte{MsgActionSealed}
	util.PutHash(action, &msg)
	util.PutUint64(block, &msg)
	util.PutHash(blockHash, &msg)
	return msg
}

func ParseSealedAction(msg []byte) (crypto.Hash, uint64, crypto.Hash) {
	if len(msg) < 1 || msg[0] != MsgActionSealed {
		return crypto.Hash{}, 0, crypto.Hash{}
	}
	position := 1
	action, position := util.ParseHash(msg, position)
	block, position := util.ParseUint64(msg, position)
	blockHash, position := util.ParseHash(msg, position)
	if position != len(msg) {
		return crypto.Hash{}, 0, crypto.Hash{}
	}
	return action, block, blockHash
}
