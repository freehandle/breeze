package gateway

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

const SendNextNValidators = 5
const TargetListenerSize = 2

type WindowValidators struct {
	Start uint64
	End   uint64
	order []*socket.SignedConnection
}

func (v *WindowValidators) GetPool(epoch uint64) []*socket.SignedConnection {
	leader := epoch % uint64(len(v.order))
	if v.End-epoch > SendNextNValidators {
		return v.order[leader : leader+SendNextNValidators]
	} else {
		return v.order[leader:]
	}
}

func uniquePeers(peers []socket.TokenAddr) []socket.TokenAddr {
	unique := make([]socket.TokenAddr, 0)
	for _, peer := range peers {
		alreadyPeer := false
		for _, u := range unique {
			if u.Token.Equal(peer.Token) {
				alreadyPeer = true
				break
			}
		}
		if !alreadyPeer {
			unique = append(unique, peer)
		}
	}
	return unique
}

type Gateway struct {
	ctx           context.Context
	ready         bool
	hostname      string
	Epoch         uint64
	credentials   crypto.PrivateKey
	wallet        crypto.PrivateKey
	relayPort     int
	allLiveConn   []*socket.SignedConnection
	fwdPool       []*socket.SignedConnection
	feedPool      *socket.Aggregator
	currentWindow *WindowValidators
	nextWindow    *WindowValidators
	trusted       []socket.TokenAddr
	store         *store.ActionStore
}

func (g *Gateway) Forward(data []byte) {
	if !g.ready {
		slog.Error("Gateway: Forward called before gateway is ready")
		return
	}
	for _, conn := range g.fwdPool {
		if conn.Live {
			if err := conn.Send(data); err != nil {
				conn.Live = false
			}
		}
	}
}

func (g *Gateway) MoveNext() {
	g.Epoch += 1
	if !g.ready || g.currentWindow == nil {
		return
	}
	if g.Epoch > g.currentWindow.End {
		if g.nextWindow == nil {
			slog.Info("Gateway: MoveNext called with no next window")
			g.ready = false
			return
		}
		g.currentWindow = g.nextWindow
		g.nextWindow = nil
		copy(g.fwdPool, g.currentWindow.GetPool(g.Epoch))
	} else {
		pool := g.currentWindow.GetPool(g.Epoch)
		if len(pool) < SendNextNValidators {
			for n, conn := range pool {
				g.fwdPool[n] = conn
			}
			if g.nextWindow != nil {
				for n := 0; n < SendNextNValidators-len(pool); n++ {
					g.fwdPool[n+len(pool)] = g.nextWindow.order[n]
				}
			} else {
				slog.Info("Gateway: incomplete forward: MoveNext close to next windows called with no next window")
			}
		} else {
			copy(g.fwdPool, pool)
		}
	}
}

func (g *Gateway) AssembleConnections(order []socket.TokenAddr) chan []*socket.SignedConnection {
	peers := uniquePeers(order)
	if len(peers) == 0 {
		void := make(chan []*socket.SignedConnection, 2)
		void <- nil
		slog.Error("Gateway: PrepareNextWindow called with no peers to connect to")
		return void
	}
	identity := func(s *socket.SignedConnection) *socket.SignedConnection { return s }
	return socket.AssembleCommittee[*socket.SignedConnection](g.ctx, peers, g.allLiveConn, identity, g.credentials, g.relayPort, g.hostname)
}

func (g *Gateway) PrepareNextWindow(order []socket.TokenAddr) {
	promise := g.AssembleConnections(order)
	go func() {
		pool := <-promise
		validators := WindowValidators{
			Start: g.currentWindow.End + 1,
			End:   g.currentWindow.End + 1 + (g.currentWindow.End - g.currentWindow.Start),
			order: make([]*socket.SignedConnection, len(order)),
		}
		for n, tk := range order {
			for _, connected := range pool {
				if connected.Is(tk.Token) {
					validators.order[n] = connected
					break
				}
			}
		}
		g.nextWindow = &validators
	}()
}

func (g *Gateway) LaunchConnections(ctx context.Context, start, end uint64, order []socket.TokenAddr) {
	participants := uniquePeers(order)
	promise := g.AssembleConnections(order)
	go func() {
		pool := <-promise
		if len(pool) == 0 {
			slog.Warn("Gateway: LaunchConnections returned with no connections")
			return
		}
		g.currentWindow = &WindowValidators{
			Start: start,
			End:   end,
			order: pool,
		}
		g.ready = true
		socket.NewTrustedAgregator(g.ctx, g.hostname, g.credentials, TargetListenerSize, g.trusted, participants, pool...)

	}()
}

func (g *Gateway) Sealed(sealed *chain.SealedBlock) {

}

func (g *Gateway) Commit(epoch uint64, hash crypto.Hash) {

}

func (g *Gateway) Window(validators []socket.TokenAddr) {

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
			epoch, hash, _ := messages.ParseEpochAndHash(data)
			if epoch > 0 {
				g.Commit(epoch, hash)
			}
		case messages.MsgNetworkTopologyResponse:
			topology := messages.ParseNetworkTopologyMessage(data)

		}
	}

}
