package chain

import (
	"github.com/freehandle/breeze/util"
)

const (
	MsgBlock byte = iota
	MsgAction
	MsgSyncRequest
	MsgActionSubmit
	MsgSyncError
	MsgNewBlock
	MsgSealBlock
	MsgCommitBlock
	MsgBlockBuilding
	MsgBlockSealed
	MsgBlockCommitted
	MsgFullBlock
	MsgProtocolNewBlock
	MsgProtocolActionArray
	MsgProtocolNewAction
	MsgProtocolSealBlock
	MsgProtocolCommitBlock
)

func BlockMessage(block []byte) []byte {
	return append([]byte{MsgBlock}, block...)
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

func NewBlockMessage(header BlockHeader) []byte {
	return append([]byte{MsgNewBlock}, header.Serialize()...)
}

func BlockSealMessage(epoch uint64, seal BlockSeal) []byte {
	bytes := []byte{MsgSealBlock}
	util.PutUint64(epoch, &bytes)
	return append(bytes, seal.Serialize()...)
}

func CommitBlockMessage(epoch uint64, commit *BlockCommit) []byte {
	bytes := []byte{MsgCommitBlock}
	util.PutUint64(epoch, &bytes)
	return append(bytes, commit.Serialize()...)
}
