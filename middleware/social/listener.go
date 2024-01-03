package social

import (
	"log"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"

	"github.com/freehandle/breeze/socket"
)

const (
	ErrSignal byte = iota
	NewBlockSignal
	ActionSignal
	ActionArraySignal
	SealSignal
	CommitSignal
)

type BlockSignal struct {
	Signal     byte
	Epoch      uint64
	Checkpoint uint64
	Hash       crypto.Hash
	HashArray  []crypto.Hash
	Token      crypto.Token
	Signature  crypto.Signature
	Action     []byte
	Actions    *chain.ActionArray
	Err        error
}

type Node struct {
	Address string
	Token   crypto.Token
}

func SocialProtocolBlockListener(address string, node crypto.Token, credentials crypto.PrivateKey, epoch uint64) chan *ProtocolBlock {
	send := make(chan *ProtocolBlock, 2)
	conn, err := socket.Dial("localhost", address, credentials, node)
	if err != nil {
		log.Printf("SocialProtocolBlockListener: could not connect to %v: %v", address, err)
		send <- nil
		return send
	}
	if err := conn.Send(NewSyncRequest(epoch)); err != nil {
		log.Printf("SocialProtocolBlockListener: could not send to %v: %v", address, err)
		send <- nil
		return send
	}
	go func() {
		blocks := make(map[uint64]*ProtocolBlock)
		defer conn.Shutdown()
		var current *ProtocolBlock
		for {
			data, err := conn.Read()
			if err != nil {
				log.Printf("SocialProtocolBlockListener: could not read from connection: %v", err)
				send <- nil
				return
			}
			switch data[0] {
			case messages.MsgNewBlock:
				epoch, err := ParseNewBlockSocial(data)
				if err != nil {
					log.Printf("SocialProtocolBlockListener: could not parse message: %v", err)
					send <- nil
					return
				}
				current = &ProtocolBlock{
					Epoch:   epoch,
					Actions: make([][]byte, 0),
				}
				blocks[epoch] = current
			case messages.MsgAction:
				if current == nil {
					log.Print("SocialProtocolBlockListener: messages out of order: action before new block signal")
					send <- nil
					return
				}
				current.Actions = append(current.Actions, data[1:])
			case messages.MsgSealedBlock:
				epoch, hash, err := ParseSealBlockSocial(data)
				if err != nil {
					log.Printf("SocialProtocolBlockListener: could not parse message: %v", err)
					send <- nil
					return
				}
				if current.Epoch == epoch {
					current = nil // no actions after seal
				}
				if block, ok := blocks[epoch]; ok {
					block.Hash = hash
				} else {
					log.Printf("SocialProtocolBlockListener: sealed unkown block: %v", epoch)
					send <- nil
					return
				}
			case messages.MsgCommittedBlock:
				epoch, invalidated, err := ParseCommitBlockSocial(data)
				if err != nil {
					log.Printf("SocialProtocolBlockListener: could not parse message: %v", err)
				}
				if block, ok := blocks[epoch]; ok {
					block.Invalidated = invalidated
					send <- block
					delete(blocks, epoch)
				} else {
					log.Printf("SocialProtocolBlockListener: commit unkown block: %v", epoch)
					send <- nil
					return
				}
			}
		}
	}()
	return send
}

// Connects to a node providing breeze new blocks and forward signals to the channel
func BreezeBlockListener(config ProtocolValidatorNodeConfig, epoch uint64) chan *BlockSignal {
	send := make(chan *BlockSignal)
	conn, err := socket.Dial("localhost", config.BlockProviderAddr, config.NodeCredentials, config.BlockProviderToken)
	if err != nil {
		signal := &BlockSignal{Signal: ErrSignal, Err: err}
		send <- signal
		return send
	}
	go func() {
		conn.Send(messages.SyncMessage(epoch))
		for {
			msg, err := conn.Read()
			if err != nil {
				signal := &BlockSignal{Signal: ErrSignal, Err: err}
				send <- signal
				return
			}
			switch msg[0] {
			case messages.MsgAction:
				signal := &BlockSignal{Signal: ActionSignal, Action: msg[1:]}
				send <- signal
			case messages.MsgNewBlock:
				header := chain.ParseBlockHeader(msg[1:])
				if header != nil {
					signal := &BlockSignal{
						Signal:     NewBlockSignal,
						Epoch:      header.Epoch,
						Checkpoint: header.CheckPoint,
						Hash:       header.CheckpointHash,
						Token:      header.Proposer,
					}
					send <- signal
				}
			case messages.MsgSeal:
				if len(msg) > 9 {
					signal := &BlockSignal{
						Signal: SealSignal,
					}
					signal.Epoch, _ = util.ParseUint64(msg, 1)
					seal := chain.ParseBlockSeal(msg[9:])
					if seal != nil {
						signal.Hash = seal.Hash
						signal.Signature = seal.SealSignature
						send <- signal
					}
				}
			case messages.MsgCommit:
				if len(msg) > 9 {
					signal := &BlockSignal{
						Signal: CommitSignal,
					}
					signal.Epoch, _ = util.ParseUint64(msg, 1)
					commit := chain.ParseBlockCommit(msg[9:])
					if commit != nil {
						signal.HashArray = commit.Invalidated
						signal.Token = commit.PublishedBy
						signal.Signature = commit.PublishSign
						send <- signal
					}
				}
			case messages.MsgCommittedBlock:
				block := chain.ParseCommitBlock(msg[1:])
				if block != nil {
					signal := &BlockSignal{
						Signal: NewBlockSignal,
					}
					signal.Epoch = block.Header.Epoch
					signal.Checkpoint = block.Header.CheckPoint
					signal.Hash = block.Header.CheckpointHash
					signal.Token = block.Header.Proposer
					send <- signal
					signal.Signal = ActionArraySignal
					signal.Actions = block.Actions
					send <- signal
					signal.Signal = SealSignal
					signal.Hash = block.Seal.Hash
					signal.Signature = block.Seal.SealSignature
					send <- signal
					signal.Signal = CommitSignal
					signal.HashArray = block.Commit.Invalidated
					signal.Token = block.Commit.PublishedBy
					signal.Signature = block.Commit.PublishSign
					send <- signal
				}
			case messages.MsgSealedBlock:
				block := chain.ParseSealedBlock(msg[1:])
				if block != nil {
					signal := &BlockSignal{
						Signal: NewBlockSignal,
					}
					signal.Epoch = block.Header.Epoch
					signal.Checkpoint = block.Header.CheckPoint
					signal.Hash = block.Header.CheckpointHash
					signal.Token = block.Header.Proposer
					send <- signal
					signal.Signal = ActionArraySignal
					signal.Actions = block.Actions
					send <- signal
					signal.Signal = SealSignal
					signal.Hash = block.Seal.Hash
					signal.Signature = block.Seal.SealSignature
					send <- signal
				}
			}
		}
	}()
	return send
}
