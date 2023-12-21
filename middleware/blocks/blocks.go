package blocks

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type Blocks struct {
	Credentials     crypto.PrivateKey
	LastCommitEpoch uint64
	SealedBlocks    []*chain.SealedBlock
	RecentBLocks    []*chain.CommitBlock
	Clock           chain.ClockSyncronization

	Aggregator *socket.Aggregator

	Statements []*chain.ChecksumStatement
}

// Incorporate adds data to the block buffer. The data should be a message
// of type MsgSealedBlock, MsgCommittedBlock or MsgCommit.
func (b *Blocks) Incorporate(data []byte) {
	if len(data) == 0 {
		slog.Warn("blocks.Incorporate: empty data received")
		return
	}
	switch data[0] {
	case messages.MsgSealedBlock:
		if sealedBlock := chain.ParseSealedBlock(data[1:]); sealedBlock != nil {
			b.SealedBlocks = append(b.SealedBlocks, sealedBlock)
			for _, statement := range sealedBlock.Header.Candidate {
				b.incorporateStatemnet(statement)
			}
		}
	case messages.MsgCommittedBlock:
		if commitBlock := chain.ParseCommitBlock(data[1:]); commitBlock != nil {
			b.RecentBLocks = append(b.RecentBLocks, commitBlock)
		}
	case messages.MsgCommit:
		epoch, hash, commitBytes := messages.ParseEpochAndHash(data[1:])
		if len(commitBytes) > 0 {
			for i, sealed := range b.SealedBlocks {
				if sealed.Header.Epoch == epoch && sealed.Seal.Hash.Equal(hash) {
					if commit := chain.ParseBlockCommit(commitBytes); commit != nil {
						block := chain.CommitBlock{
							Header:  sealed.Header,
							Actions: sealed.Actions,
							Seal:    sealed.Seal,
							Commit:  commit,
						}
						b.RecentBLocks = append(b.RecentBLocks, &block)
						if epoch > b.LastCommitEpoch {
							b.LastCommitEpoch = epoch

						}
						b.SealedBlocks = append(b.SealedBlocks[:i], b.SealedBlocks[i+1:]...)
					}
					break
				}
			}
		}
	case messages.MsgNetworkTopologyResponse:
		order, members := swell.ParseCommitee(data)
		if len(order) > 0 {

		}
	}
}

func (b *Blocks) incorporateStatemnet(statement *chain.ChecksumStatement) {
	if statement == nil {
		return
	}
	// try to find a previous statement (same node and epoch)
	for _, existing := range b.Statements {
		if statement.Node.Equal(existing.Node) && statement.Epoch == existing.Epoch {
			// only a naked statement with a previously dressed statement is valid
			if existing.Naked || !statement.Naked {
				return
			}
			// and dressed and naked hash must match
			hash := crypto.Hasher(append(existing.Node[:], statement.Hash[:]...))
			if existing.Hash.Equal(hash) {
				b.Statements = append(b.Statements, statement)
			}
			return
		}
	}
	// cannot send a naked without a dressed
	if statement.Naked {
		return
	}
	b.Statements = append(b.Statements, statement)
}

func NewBlocks(ctx context.Context, hostname string, credentials crypto.PrivateKey, provider socket.TokenAddr) *Blocks {
	blocks := &Blocks{
		Credentials:  credentials,
		SealedBlocks: []*chain.SealedBlock{},
		RecentBLocks: []*chain.CommitBlock{},
		Aggregator:   socket.NewAgregator(ctx, hostname, credentials),
	}
	if blocks.Aggregator == nil {
		return nil
	}
	blocks.Aggregator.AddProvider(provider)

	go func() {
		for {
			data, err := blocks.Aggregator.Read()
			if err != nil {
				return
			}
			blocks.Incorporate(data)
		}
	}()
	return blocks
}
