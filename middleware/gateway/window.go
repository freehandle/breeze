package gateway

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
)

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

/*func uniquePeers(peers []socket.TokenAddr) []socket.TokenAddr {
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
*/

func LaunchWindow(ctx context.Context, config Configuration, start, end uint64, order []crypto.Token, validators []socket.TokenAddr) chan *WindowValidators {
	finished := make(chan *WindowValidators, 2)
	if len(validators) == 0 {
		slog.Error("Gateway: LaunchWindow called with no validators to connect to")
		finished <- nil
		return finished
	}
	identity := func(s *socket.SignedConnection) *socket.SignedConnection { return s }
	promise := socket.AssembleCommittee[*socket.SignedConnection](ctx, validators, nil, identity, config.Credentials, config.ActionRelayPort, config.Hostname)
	go func() {
		pool := <-promise
		if len(pool) == 0 {
			slog.Warn("Gateway: LaunchConnections returned with no connections")
			finished <- nil
			return
		}
		window := &WindowValidators{
			Start: start,
			End:   end,
			order: make([]*socket.SignedConnection, len(order)),
		}
		for n, token := range order {
			for _, connected := range pool {
				if connected.Is(token) {
					window.order[n] = connected
					break
				}
			}
		}
		finished <- window
	}()
	return finished
}
