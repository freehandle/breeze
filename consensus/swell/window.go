package swell

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/freehandle/breeze/consensus/bft"
	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
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
			StartNonValidatorEngine(next, conn, true)
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
	for n := 0; n < sealed.Actions.Len(); n++ {
		action := sealed.Actions.Get(n)
		hash := crypto.Hasher(action)
		w.Node.actions.Exlude(hash)
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

func (w *Window) CanPublishStatement(epoch uint64) bool {
	return epoch > w.Start+(w.End-w.Start)*8/10 && epoch <= w.Start+(w.End-w.Start)*9/10
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

// Node keeps forming blocks either proposing its own blocks or validating
// others nodes proposals. In due time node re-arranges validator pool.
// Uppon exclusion a node can transition to a listener node.
func (w *Window) RunEpoch(epoch uint64) {
	leaderCount := int(epoch-w.Start) % len(w.Committee.order)
	leaderToken := w.Committee.order[leaderCount]
	poolingCommittee := &bft.PoolingCommittee{
		Epoch:   epoch,
		Members: make(map[crypto.Token]bft.PoolingMembers),
		Order:   make([]crypto.Token, 0),
	}
	peers := make([]socket.TokenAddr, 0)
	for i := 0; i < w.Node.config.MaxPoolSize; i++ {
		token := w.Committee.order[(leaderCount+i)%len(w.Committee.order)]
		weight := w.Committee.weights[token]
		if weight == 0 {
			slog.Warn("RunEpoch: zero weight member")
			continue
		}
		poolingCommittee.Order = append(poolingCommittee.Order, token)
		if member, ok := poolingCommittee.Members[token]; ok {
			poolingCommittee.Members[token] = bft.PoolingMembers{Weight: member.Weight + weight}
		} else {
			poolingCommittee.Members[token] = bft.PoolingMembers{Weight: weight}
			for _, v := range w.Committee.validators {
				if v.Token.Equal(token) {
					peers = append(peers, v)
					break
				}
			}
		}
	}
	bftConnections := socket.AssembleChannelNetwork(w.ctx, peers, w.Node.credentials, 5401, w.Node.hostname, w.Committee.consensus)
	poolingCommittee.Gossip = socket.GroupGossip(epoch, bftConnections)

	pool := bft.LaunchPooling(*poolingCommittee, w.Node.credentials)
	go func() {
		ok := false
		if leaderToken.Equal(w.Node.credentials.PublicKey()) {
			ok = w.BuildBlock(epoch, pool)
		} else {
			leader, others := w.Committee.blocks.GetLeader(leaderToken)
			if leader != nil {
				ok = w.ListenToBlock(leader, others, pool)
			}
		}
		w.newBlock <- BlockConsensusConfirmation{Epoch: epoch, Status: ok}
	}()
}

// BuildSoloBLock builds a block in the case where the node is the sole partipant
// in the validating network. In this case all the extra burden can be eliminated
// and the node can build, seal, commit and broadcast a block in a single step.
func (w *Window) BuildSoloBLock(epoch uint64) bool {
	timeout := time.NewTimer(980 * w.Node.config.BlockInterval / 1000)
	block := w.StartNewBlock(epoch)
	if block == nil {
		return false
	}
	for {
		select {
		case action := <-w.Node.actions.Pop:
			if len(action.Data) > 0 && block.Validate(action.Data) {
				// clear actionarray
			}
		case <-timeout.C:
			sealed := block.Seal(w.Node.credentials)
			if sealed == nil {
				slog.Warn("BuildBlock: could not seal own block")
				return false
			} else {
				w.AddSealedBlock(sealed)
				w.Node.relay.BlockEvents <- messages.SealedBlock(sealed.Serialize())
				return true
			}
		}
	}
}

// BuildBlock build a new block according to the available state of the swell
// node at the calling of this method. The block is broadcasted to the gossip
// network and the pool consensus committee is launched. Once terminated the
// node cast a proposal for the given hash on the pool network.
func (w *Window) BuildBlock(epoch uint64, pool *bft.Pooling) bool {
	timeout := time.NewTimer(980 * time.Millisecond / 1000)
	block := w.StartNewBlock(epoch)
	if block == nil {
		return false
	}
	msg := messages.NewBlockMessage(block.Header.Serialize())
	w.Committee.blocks.Send(epoch, msg)
	var sealed *chain.SealedBlock
	go func() {
		for {
			select {
			case action := <-w.Node.actions.Pop:
				fmt.Println("****", action)
				if len(action.Data) > 0 && block.Validate(action.Data) {
					msg := messages.ActionMessage(action.Data)
					w.Committee.blocks.Send(epoch, msg)
				}
			case <-timeout.C:
				sealed = block.Seal(w.Node.credentials)
				hash := crypto.ZeroHash
				if sealed != nil {
					hash = sealed.Seal.Hash
					msg := messages.BlockSealMessage(epoch, sealed.Seal.Serialize())
					w.Committee.blocks.Send(epoch, msg)
				} else {
					slog.Warn("BuildBlock: could not seal own block")
				}
				pool.SealBlock(hash, w.Node.credentials.PublicKey())
				return
			}
		}
	}()
	consensus := <-pool.Finalize
	if sealed != nil && consensus.Value.Equal(sealed.Seal.Hash) {
		sealed.Seal.Consensus = consensus.Rounds
		w.AddSealedBlock(sealed)
		return true
	} else if consensus.Value.Equal(crypto.ZeroHash) {
		return false
	}
	return false
}

// ListenToBlock listens to the block events from the gossip network and upon
// receiving a swal informs the pool consensus committee about the hash of the
// proposed block. If the pool returns a valid consensus the block is added as
// a sealed block to the node. In case the swell node is not in posession of a
// block with the consensus hash it tries to get that block from other nodes
// of the gossip network.
func (w *Window) ListenToBlock(leader *socket.BufferedMultiChannel, others []*socket.BufferedMultiChannel, pool *bft.Pooling) bool {
	defer leader.Release(pool.Epoch())
	var sealed *chain.SealedBlock
	epoch := pool.Epoch()
	go func() {
		var block *chain.BlockBuilder
		for {
			data := leader.Read(epoch)
			if len(data) == 0 {
				continue
			}
			switch data[0] {
			case messages.MsgNewBlock:
				header := chain.ParseBlockHeader(data[1:])
				if header == nil {
					slog.Info("ListenToBlock: invalid block header")
					return
				}
				block = w.Node.blockchain.CheckpointValidator(*header)
				if block == nil {
					slog.Info("ListenToBlock: invalid block header")
					pool.SealBlock(crypto.ZeroHash, crypto.ZeroToken)
					return
				}
			case messages.MsgSeal:
				epoch, position := util.ParseUint64(data, 1)
				seal := chain.ParseBlockSeal(data[position:])
				if seal == nil {
					slog.Info("ListenToBlock: invalid seal", "epoch", epoch)
					return
				}
				if block == nil {
					slog.Info("ListenToBlock: received seal without block header", "epoch", epoch, "seal", crypto.EncodeHash(seal.Hash))
					return
				}
				if epoch != block.Header.Epoch {
					slog.Info("ListenToBlock: received seal incompatible with block header epoch")
					return
				}
				sealed = block.ImprintSeal(*seal)
				pool.SealBlock(seal.Hash, block.Header.Proposer)
				return
			case messages.MsgAction:
				if block != nil {
					if !block.Validate(data[1:]) {
						slog.Info("ListenToBlock: invalid action")
					}
				}
			}
		}
	}()
	consensus := <-pool.Finalize

	if consensus == nil {
		slog.Error("ListenToBlock: nil consensus received from channel")
		return false
	}

	if sealed == nil || (!consensus.Value.Equal(sealed.Seal.Hash)) {
		nodesWithData := make(map[crypto.Token]struct{})
		for _, round := range consensus.Rounds {
			for _, vote := range round.Votes {
				if vote.HasHash && vote.Value.Equal(consensus.Value) {
					nodesWithData[vote.Token] = struct{}{}
				}
			}
		}
		order := make([]*socket.BufferedMultiChannel, 0)
		for token := range nodesWithData {
			for _, others := range others {
				if others.Is(token) {
					order = append(order, others)
					break
				}
			}
		}
		sealed = <-RetrieveBlock(pool.Epoch(), consensus.Value, order)
	}
	if sealed == nil {
		slog.Warn("Breeze: ListentToBlock could not retrieve sealed block compatible consensus")
		return false
	}
	sealed.Seal.Consensus = consensus.Rounds
	w.AddSealedBlock(sealed)
	return true
}
