package chain

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

const (
	MsgBlock               byte = iota // breeze protocol new block with heder
	MsgAction                          // sinsgle action
	MsgActionArray                     // multiple actions
	MsgSyncRequest                     // Request Syncrhonization starting at given epoch
	MsgActionSubmit                    // Submit action to the network
	MsgSyncError                       // Cannot synchronize
	MsgNewBlock                        // Breeze new block with header
	MsgSealBlock                       // Breeze Seal Block with seal
	MsgCommitBlock                     // Breeze Node own Commit Block with invalidated
	MsgBlockBuilding                   // Block under constructions
	MsgBlockSealed                     // Block Seal
	MsgBlockCommitted                  // Block Commit
	MsgFullBlock                       // EntireBlockMessage
	MsgProtocolNewBlock                // Sub-Protocol New Block Message
	MsgProtocolActionArray             // Sub-Protocol Action Array Message
	MsgProtocolNewAction               // Sub-Protocol New Action Message
	MsgProtocolSealBlock               // Sub-Protool Seal Block Message
	MsgProtocolCommitBlock             // Sub-Protocol Commit Block Message
	MsgProtocolFullBlock               // Sub-Protocol Full Block Message
	MsgRequestBlock                    // Request a block
)

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
