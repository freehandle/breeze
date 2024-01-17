package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/swell"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

const SendNextNValidators = 5
const TargetListenerSize = 2

type ClockSync struct {
	SyncEpoch     uint64
	SyncEpochTime time.Time
	BlockInterval time.Duration
	Epoch         uint64
	Timer         *time.Timer
}

func (c *ClockSync) reset() {
	c.Epoch += 1
	count := c.Epoch - c.SyncEpoch
	nextEpoch := c.SyncEpochTime.Add((time.Duration(count) + 1) * c.BlockInterval)
	c.Timer.Reset(time.Until(nextEpoch))
}

func NewClockSyn(start uint64, at time.Time, blockInterval int) *ClockSync {
	interval := time.Duration(blockInterval) * time.Millisecond
	epochs := uint64(time.Since(at) / interval)
	nextEpoch := at.Add((time.Duration(epochs) + 1) * interval)
	return &ClockSync{
		SyncEpoch:     start,
		SyncEpochTime: at,
		BlockInterval: interval,
		Epoch:         start + epochs,
		Timer:         time.NewTimer(time.Until(nextEpoch)),
	}
}

type Gateway struct {
	ctx              context.Context
	config           Configuration
	ready            bool
	sync             *ClockSync
	liveActionRelays []*socket.SignedConnection
	activeFwdPool    []*socket.SignedConnection
	feedPool         *socket.TrustedAggregator
	currentWindow    *WindowValidators
	nextWindow       *WindowValidators
	sealedBlocks     map[uint64]*chain.SealedBlock
	store            *ActionVault
}

func LaunchGateway(ctx context.Context, config Configuration, trusted *socket.SignedConnection, topology *messages.NetworkTopology, propose chan *Propose) {
	clock := NewClockSyn(topology.Start, topology.StartAt, config.Breeze.BlockInterval)
	gateway := Gateway{
		ctx:              ctx,
		config:           config,
		sync:             clock,
		liveActionRelays: make([]*socket.SignedConnection, 0),
		activeFwdPool:    make([]*socket.SignedConnection, 0),
		store:            NewActionVault(ctx, clock.Epoch, propose),
	}

	windowReady := LaunchWindow(ctx, config, topology.Start, topology.End, topology.Order, topology.Validators)
	gateway.feedPool = socket.NewTrustedAgregator(ctx, config.Hostname, config.Credentials, TargetListenerSize, config.Trusted, topology.Validators, trusted)
	gateway.ready = false
	go func() {
		done := ctx.Done()
		for {
			if !gateway.ready {
				select {
				case window := <-windowReady:
					gateway.currentWindow = window
					gateway.activeFwdPool = window.GetPool(gateway.sync.Epoch)
					fmt.Println("Gateway: current window", len(gateway.activeFwdPool))
					for _, conn := range window.order {
						isNew := true
						for _, live := range gateway.liveActionRelays {
							if live == conn {
								isNew = false
								break
							}
						}
						if isNew {
							gateway.liveActionRelays = append(gateway.liveActionRelays, conn)
						}
					}
					close(windowReady)
					gateway.ready = true
					fmt.Println("Gateway: ready")
				case <-gateway.sync.Timer.C:
					gateway.NextBlock()
				case conn := <-gateway.feedPool.Activate:
					conn.Send([]byte{messages.MsgSubscribeBlockEvents})
				}
			} else {
				select {
				case <-done:
					return
				case <-gateway.sync.Timer.C:
					gateway.NextBlock()
				case bytes := <-gateway.store.Pop:
					gateway.Forward(bytes)
				case conn := <-gateway.feedPool.Activate:
					conn.Send([]byte{messages.MsgSubscribeBlockEvents})
				}
			}

		}
	}()
}

func (g *Gateway) Forward(data []byte) {
	if !g.ready {
		slog.Error("Gateway: Forward called before gateway is ready")
		return
	}
	for _, conn := range g.activeFwdPool {
		if conn.Live {
			fmt.Println("Gateway: Forwarding to", conn.Address, conn.Token)
			if err := conn.Send(data); err != nil {
				conn.Live = false
			}
		}
	}
}

func (g *Gateway) NextBlock() {
	g.sync.reset()
	g.store.NextEpoch()
	slog.Info("Gateway: NextBlock called", "clock", g.sync.Epoch)
	if !g.ready || g.currentWindow == nil {
		return
	}
	if g.sync.Epoch > g.currentWindow.End {
		if g.nextWindow == nil {
			slog.Info("Gateway: MoveNext called with no next window")
			g.ready = false
			return
		}
		g.currentWindow = g.nextWindow
		g.nextWindow = nil
		copy(g.activeFwdPool, g.currentWindow.GetPool(g.sync.Epoch))
	} else {
		pool := g.currentWindow.GetPool(g.sync.Epoch)
		if len(pool) < SendNextNValidators {
			for n, conn := range pool {
				g.activeFwdPool[n] = conn
			}
			if g.nextWindow != nil {
				for n := 0; n < SendNextNValidators-len(pool); n++ {
					g.activeFwdPool[n+len(pool)] = g.nextWindow.order[n]
				}
			} else {
				slog.Info("Gateway: incomplete forward: MoveNext close to next windows called with no next window")
			}
		} else {
			copy(g.activeFwdPool, pool)
		}
	}
}

func (g *Gateway) AssembleConnections(validators []socket.TokenAddr) chan []*socket.SignedConnection {
	if len(validators) == 0 {
		void := make(chan []*socket.SignedConnection, 2)
		void <- nil
		slog.Error("Gateway: PrepareNextWindow called with no peers to connect to")
		return void
	}
	identity := func(s *socket.SignedConnection) *socket.SignedConnection { return s }
	return socket.AssembleCommittee[*socket.SignedConnection](g.ctx, validators, g.liveActionRelays, identity, g.config.Credentials, g.config.ActionRelayPort, g.config.Hostname)
}

func (g *Gateway) PrepareNextWindow(order []crypto.Token, validators []socket.TokenAddr) {
	promise := g.AssembleConnections(validators)
	go func() {
		pool := <-promise
		validators := WindowValidators{
			Start: g.currentWindow.End + 1,
			End:   g.currentWindow.End + 1 + (g.currentWindow.End - g.currentWindow.Start),
			order: make([]*socket.SignedConnection, len(order)),
		}
		for n, token := range order {
			for _, connected := range pool {
				if connected.Is(token) {
					validators.order[n] = connected
					break
				}
			}
		}
		g.nextWindow = &validators
	}()
}

func (g *Gateway) Sealed(sealed *chain.SealedBlock) {
	for n := 0; n < sealed.Actions.Len(); n++ {
		action := sealed.Actions.Get(n)
		g.store.seal <- SealOnBlock{
			Action:    action,
			Epoch:     sealed.Header.Epoch,
			BlockHash: sealed.Seal.Hash,
		}
	}
}

func (g *Gateway) Commit(epoch uint64, hash crypto.Hash, commit *chain.BlockCommit) {
	if commit == nil {
		slog.Error("Gateway: Commit called with nil commit")
		return
	}
	sealed := g.sealedBlocks[epoch]
	if sealed == nil {
		slog.Error("Gateway: Commit called with no sealed block")
		return
	}
	invalid := make(map[crypto.Hash]struct{})
	for _, hash := range commit.Invalidated {
		invalid[hash] = struct{}{}
	}
	for n := 0; n < sealed.Actions.Len(); n++ {
		action := sealed.Actions.Get(n)
		hash := crypto.Hasher(action)
		if _, ok := invalid[hash]; !ok {
			g.store.commit <- hash
		}
	}
	// only commit once
	delete(g.sealedBlocks, epoch)
}

func (g *Gateway) Blockfeed() {

	for {
		data, err := g.feedPool.Read()
		if err != nil {
			slog.Warn("Blockfeed: error reading the validator sample pool", "error", err)
		}
		switch data[0] {
		case messages.MsgSealedBlock:
			sealed := chain.ParseSealedBlock(data[1:])
			if sealed != nil {

			}
		case messages.MsgCommit:
			epoch, hash, bytes := messages.ParseEpochAndHash(data)
			if sealed, ok := g.sealedBlocks[epoch]; ok {
				if sealed.Seal.Hash.Equal(hash) {
					commit := chain.ParseBlockCommit(bytes)
					if commit != nil {
						g.Commit(epoch, hash, commit)
					}
				}
			}
		case messages.MsgNextCommittee:
			order, validators := swell.ParseCommitee(data[1:])
			g.PrepareNextWindow(order, validators)
		}
	}

}
