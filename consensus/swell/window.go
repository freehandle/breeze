package swell

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

type WindowWithValidators struct {
	window     *Window
	validators []socket.TokenAddr
}

// Window is a given sequence of blocks from Start to End under responsability of
// the Committee to produce blocks for the SwellNode instance
type Window struct {
	Start           uint64
	End             uint64
	Committee       *Committee
	Node            *SwellNode
	newBlock        chan BlockConsensusConfirmation
	candidate       CandidateStatus
	unpublished     []*chain.ChecksumStatement
	published       []*chain.ChecksumStatement
	hasPreparedNext bool
	nextListener    chan *WindowWithValidators
	nextCommittee   *Committee
	ctx             context.Context
}

func (w *Window) Finished() bool {
	return w.End <= w.Node.blockchain.LastCommitEpoch
}

func (w *Window) incorporateStatement(statement *chain.ChecksumStatement, epoch uint64) {
	if statement == nil {
		return
	}
	for _, published := range w.published {
		if published.Node.Equal(statement.Node) && published.Epoch == statement.Epoch {
			if published.Naked {
				slog.Error("Window.incorporateStatement: found naked before dressed", "node", statement.Node, "epoch", statement.Epoch)
				return
			}
			if !statement.Naked {
				// only the first dressed and the first compatible naked are incorporated
				return
			}
			hash := crypto.Hasher(append(published.Node[:], statement.Hash[:]...))
			if published.Hash.Equal(hash) {
				slog.Info("Window: new naked checksum published", "hostname", w.Node.hostname, "node", statement.Node, "epoch", statement.Epoch, "hash", crypto.EncodeHash(statement.Hash), "block", epoch)
				w.published = append(w.published, statement)
			} else {
				slog.Error("Window.incorporateStatement: incompatible naked", "node", statement.Node, "epoch", statement.Epoch)
			}
			return
		}
	}
	if !statement.Naked {
		slog.Info("Widow: new dressed checksum published", "hostname", w.Node.hostname, "node", statement.Node, "epoch", statement.Epoch, "hash", crypto.EncodeHash(statement.Hash), "block", epoch)
		w.published = append(w.published, statement)
	}
}

func (w *Window) StartNewBlock(epoch uint64) *chain.BlockBuilder {
	header := w.Node.blockchain.NextBlock(epoch)
	if header == nil {
		slog.Warn("Blockchain: breeze StartNewBlock could not form new Header", "epoch", epoch)
		return nil
	}
	header.Candidate = []*chain.ChecksumStatement{}
	if statement := w.DressedChecksumStatement(epoch); statement != nil {
		header.Candidate = append(header.Candidate, statement)
	} else if statement := w.NakedChecksumWindow(epoch); statement != nil {
		header.Candidate = append(header.Candidate, statement)
	}
	header.Candidate = append(header.Candidate, w.unpublished...)
	w.unpublished = w.unpublished[:0]
	block := w.Node.blockchain.CheckpointValidator(*header)
	return block
}

func (w *Window) PrepareNewWindow() {
	next := &Window{
		ctx:         w.ctx,
		Start:       w.End + 1,
		End:         w.End + 1 + (w.End - w.Start),
		Node:        w.Node,
		newBlock:    make(chan BlockConsensusConfirmation),
		candidate:   CandidateStatus{},
		unpublished: make([]*chain.ChecksumStatement, 0),
		published:   make([]*chain.ChecksumStatement, 0),
	}

	consenusHash, ok := getConsensusHash(w.published, w.Committee.weights)
	if !ok {
		slog.Warn("PrepareNewWindow: could not find consensus hash", "start", next.Start)
		for _, p := range w.published {
			fmt.Println(p)
		}
		// TODO: how to handle this?
		return
	}
	// preCandidates = correct naked statements but before permission
	preCandidates := make([]crypto.Token, 0)
	for _, statement := range w.published {
		if statement.Naked && statement.Hash.Equal(consenusHash) {
			preCandidates = append(preCandidates, statement.Node)
		}
	}
	// permissioned = preCandidates after permission winth permissioned
	permissioned := w.Node.config.Permission.DeterminePool(w.Node.blockchain, preCandidates)
	// candidates = permissioned sorted by swell committee rule
	candidates := sortCandidates(permissioned, consenusHash[:], w.Node.config.MaxCommitteeSize)

	aproved := make(map[crypto.Token]int)
	for _, token := range candidates {
		aproved[token] += 1
	}

	slog.Info("Breeze: next window validator pool defined", "window start", next.Start, "validators", aproved)

	// test if node is amond selected candidates
	amIIn := false
	myToken := w.Node.credentials.PublicKey()
	for _, candidate := range candidates {
		if candidate.Equal(myToken) {
			amIIn = true
			break
		}
	}
	validators := make([]socket.TokenAddr, 0)
	for _, token := range candidates {
		for _, statement := range w.published {
			if statement.Naked && statement.Node.Equal(token) {
				validators = append(validators, socket.TokenAddr{
					Addr:  statement.Address,
					Token: token,
				})
			}
		}
	}

	// if there is a listener (like in a standby node) send the new window
	// and return
	if w.nextListener != nil {
		w.nextListener <- &WindowWithValidators{
			window:     next,
			validators: validators,
		}
		return
	}

	// launch committee
	go func() {
		if !amIIn {
			slog.Info("Swell: PrepareNewWindow node not included in the committee", "start", next.Start)
			conn := ConnectRandomValidator(w.Node.hostname, w.Node.credentials, validators)
			if conn == nil {
				slog.Info("Swell: PrepareNewWindow node not connect to any member of the committee. Shutting down.")
				return
			}
			slog.Info("Swell: PrepareNewWindow starting non validator node connected to", "node", conn.Token, "start", next.Start)
			RunNonValidatorNode(next, conn, true)
			return
		}
		if w.Committee != nil {
			next.Committee = w.Committee.PrepareNext(validators)
		} else {
			next.Committee = LaunchValidatorPool(w.ctx, validators, w.Node.credentials, w.Node.hostname)
		}
		if next.Committee == nil {
			slog.Warn("Swell: PrepareNewWindow could not launch validator pool", "start", next.Start)
			return
		}
		slog.Info("Swell: PrepareNewWindow validator pool launched", "start", next.Start)
		w.nextCommittee = next.Committee
		RunValidator(next)
	}()
}

// AddSealedBlock incorporates a sealed block into the node's blockchain.
func (w *Window) AddSealedBlock(sealed *chain.SealedBlock) {
	for _, statement := range sealed.Header.Candidate {
		w.incorporateStatement(statement, sealed.Header.Epoch)
	}
	w.Node.blockchain.AddSealedBlock(sealed)
	if !w.hasPreparedNext && w.CanPrepareNextWindow() {
		w.PrepareNewWindow()
		w.hasPreparedNext = true
	}
}

func (w *Window) CanPrepareNextWindow() bool {
	epoch := w.Node.blockchain.LastCommitEpoch
	return epoch >= w.Start+(w.End-w.Start)*9/10
}

func (w *Window) DressedChecksumStatement(epoch uint64) *chain.ChecksumStatement {
	window := w.End - w.Start
	if w.candidate.Dressed || w.Node.blockchain.NextChecksum == nil || epoch <= w.Start+window/2 || epoch >= w.Start+8*window/10 {
		return nil
	}
	checkEpoch := (w.Start + w.End) / 2
	token := w.Node.credentials.PublicKey()
	dressed := crypto.Hasher(append(token[:], w.Node.blockchain.NextChecksum.Hash[:]...))
	w.candidate.Dressed = true
	return chain.NewCheckSum(checkEpoch, w.Node.credentials, w.Node.hostname, false, dressed)
}

func (w *Window) NakedChecksumWindow(epoch uint64) *chain.ChecksumStatement {
	window := w.End - w.Start
	if w.candidate.Naked || w.Node.blockchain.NextChecksum == nil || epoch < w.Start+8*window/10 || epoch >= w.Start+9*window/10 {
		return nil
	}
	checkEpoch := (w.Start + w.End) / 2
	w.candidate.Naked = true
	return chain.NewCheckSum(checkEpoch, w.Node.credentials, w.Node.hostname, true, w.Node.blockchain.NextChecksum.Hash)
}

// IsPoolMember returns true if the node is a member of the current consensus
// committe for the given epoch.
func (w *Window) IsPoolMember(epoch uint64) bool {
	token := w.Node.credentials.PublicKey()
	windowStart := int(w.Start)
	leader := int(epoch-uint64(windowStart)) % len(w.Committee.order)
	for n := 0; n < w.Node.config.MaxCommitteeSize; n++ {
		// n-th cirular token given order
		nth := w.Committee.order[(leader+n)%len(w.Committee.order)]
		if token.Equal(nth) {
			return true
		}
	}
	return false
}

func (c Window) CanPublishStatement(epoch uint64) bool {
	return epoch > c.Start+(c.End-c.Start)*8/10 && epoch <= c.Start+(c.End-c.Start)*9/10
}

// getConsensus tries to fiend a 2/3 + 1 consensus over a hash among members
func getConsensusHash(statements []*chain.ChecksumStatement, members map[crypto.Token]int) (crypto.Hash, bool) {
	totalweight := 0
	for _, weight := range members {
		totalweight += weight
	}
	weightPerHash := make(map[crypto.Hash]int)
	for _, statement := range statements {
		if statement.Naked {
			weight := weightPerHash[statement.Hash] + members[statement.Node]
			weightPerHash[statement.Hash] = weight

			if weight > 2*totalweight/3 {
				return statement.Hash, true
			}
		}
	}
	return crypto.ZeroHash, false
}
