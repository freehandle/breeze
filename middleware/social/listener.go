package social

import (
	"context"
	"fmt"
	"log"
	"log/slog"

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

type Node struct {
	Address string
	Token   crypto.Token
}

type socialListener struct {
	blocks  chan *SocialBlock
	commits chan *SocialBlockCommit
	code    uint32
}

func (s *socialListener) Apply(msg []byte) {
	switch msg[0] {
	case messages.MsgProtocolBlock:
		if block := ParseSocialBlock(msg[1:]); block != nil {
			s.blocks <- block
		}
	case messages.MsgProtocolBlockCommit:
		if commit := ParseSocialBlockCommit(msg[1:]); commit != nil {
			s.commits <- commit
		}
	}
}

func (s *socialListener) Subscribe() []byte {
	bytes := []byte{messages.MsgProtocolSubscribe}
	util.PutUint32(s.code, &bytes)
	return bytes
}

func (s *socialListener) Close() {
	close(s.blocks)
	close(s.commits)
}

type breezeListener struct {
	blocks  chan *SocialBlock
	commits chan *SocialBlockCommit
	code    uint32
}

func (b *breezeListener) Apply(msg []byte) {
	switch msg[0] {
	case messages.MsgSealedBlock:
		sealed := chain.ParseSealedBlock(msg[1:])
		fmt.Println("sealed", sealed.Header.Epoch)
		if sealed != nil {
			social := BreezeSealedBlockToSocialBlock(sealed)
			if social == nil {
				slog.Error("BreezeBlockListener: return nil")
				return
			}
			b.blocks <- social
		}
	case messages.MsgCommit:
		epoch, hash, bytes := messages.ParseEpochAndHash(msg[1:])
		commit := chain.ParseBlockCommit(bytes)
		fmt.Println("commit", epoch)
		if commit == nil {
			return
		}
		social := &SocialBlockCommit{
			ProtocolCode:    b.code,
			Epoch:           epoch,
			Publisher:       commit.PublishedBy,
			SealHash:        hash,
			Invalidated:     commit.Invalidated,
			CommitHash:      hash,
			CommitSignature: commit.PublishSign,
		}
		b.commits <- social

	case messages.MsgCommittedBlock:
		committed := chain.ParseCommitBlock(msg[1:])
		fmt.Println("committed", committed.Header.Epoch)
		if committed != nil {
			social := BreezeComiittedBlockToSocialBlock(committed)
			if social == nil {
				slog.Error("BreezeComiittedBlockToSocialBlock: return nil")
				return
			}
			b.blocks <- social
		}
	}
}

func (b *breezeListener) Subscribe() []byte {
	return []byte{messages.MsgSubscribeBlockEvents}
}

func (b *breezeListener) Close() {
	close(b.blocks)
	close(b.commits)
}

type listener interface {
	Apply([]byte)
	Subscribe() []byte
	Close()
}

func SocialProtocolBlockListener(ctx context.Context, parentProtocolCode uint32, sources *socket.TrustedAggregator, blocks chan *SocialBlock, commits chan *SocialBlockCommit) {
	var listener listener
	if parentProtocolCode == 0 {
		listener = &breezeListener{
			blocks:  blocks,
			commits: commits,
		}
	} else {
		listener = &socialListener{
			blocks:  blocks,
			commits: commits,
			code:    parentProtocolCode,
		}
	}
	withCancel, cancel := context.WithCancel(ctx)

	go func() {
		for {
			data, err := sources.Read()
			fmt.Println("data", data[0:10], err)
			if err != nil {
				log.Printf("SocialProtocolBlockListener: could not read from connection: %v", err)
				listener.Close()
				cancel()
				return
			}
			if len(data) > 0 {
				listener.Apply(data)
			}
		}
	}()

	go func() {
		defer sources.Shutdown()
		done := withCancel.Done()
		for {
			select {
			case <-done:
				return
			case conn := <-sources.Activate:
				bytes := listener.Subscribe()
				conn.Send(bytes)
			}
		}
	}()
}
