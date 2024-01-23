package social

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

func LaunchNodeFromState[M Merger[M], B Blocker[M]](ctx context.Context, cfg Configuration, checksum *Checksum[M, B], clock chain.ClockSyncronization) chan error {
	return launchNodeFromStateWithConnection[M, B](ctx, cfg, checksum, clock, nil)
}

func launchNodeFromStateWithConnection[M Merger[M], B Blocker[M]](ctx context.Context, cfg Configuration, checksum *Checksum[M, B], clock chain.ClockSyncronization, existingConnection *socket.SignedConnection) chan error {
	finalize := make(chan error, 2)

	outgoing, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Hostname, cfg.BlocksTargetPort))
	if err != nil {
		finalize <- fmt.Errorf("could not listen on port %v: %v", cfg.BlocksTargetPort, err)
		return finalize
	}
	/*conn, checksum, clock, err := SyncSocialState[M, B](cfg, newState)
	if err != nil {
		finalize <- fmt.Errorf("could not sync state: %v", err)
		return finalize
	}
	*/

	ticker := time.NewTicker(clock.Timer(cfg.RootBlockInterval))

	blockchain := NewSocialBlockChain[M, B](cfg, checksum)
	if blockchain == nil {
		finalize <- fmt.Errorf("could not create blockchain")
		return finalize
	}
	engine := NewEngine[M, B](ctx, blockchain)
	sources := socket.NewTrustedAgregator(ctx, cfg.Hostname, cfg.Credentials, cfg.ProvidersSize, cfg.TrustedProviders, nil, existingConnection)

	// connect sources to blockchain engine
	SocialProtocolBlockListener(ctx, cfg.ParentProtocolCode, sources, engine.block, engine.commit)

	syncRequest := make(chan BlockSyncRequest)

	// Listen incoming connections and accept sync requests
	// Clock synchronization is sent to any new request.
	// WaitForOutgoingSyncRequest is called to handle the request intentioin.
	go func() {
		for {
			if conn, err := outgoing.Accept(); err == nil {
				trustedConn, err := socket.PromoteConnection(conn, cfg.Credentials, cfg.Firewall)
				if err != nil {
					conn.Close()
				}
				bytes := []byte{messages.MsgClockSync}
				util.PutUint64(blockchain.Clock.Epoch, &bytes)
				util.PutTime(blockchain.Clock.TimeStamp, &bytes)
				if err := trustedConn.Send(bytes); err != nil {
					trustedConn.Shutdown()
					continue
				}
				go WaitForOutgoingSyncRequest(trustedConn, syncRequest)
			}
		}
	}()

	// connection pool loop: receive new connections, drop dead and broadcast
	go func() {
		pool := make(socket.ConnectionPool)
		for {
			select {
			case <-ticker.C:
				pool.DropDead() // clear dead connections
				ticker.Reset(clock.Timer(cfg.RootBlockInterval))
			case msg := <-engine.forward:
				pool.Broadcast(msg)
				//fmt.Println(len(pool), msg)
			case req := <-syncRequest:
				cached := socket.NewCachedConnection(req.conn)
				pool.Add(cached)
				if req.state {
					blockchain.StateSync(cached)
				} else if req.epoch > 0 {
					blockchain.SyncBlocks(cached, req.epoch)
				} else {
					cached.Ready()
				}
			}
		}

	}()

	return finalize

}

func LaunchSyncNode[M Merger[M], B Blocker[M]](ctx context.Context, cfg Configuration, peers []socket.TokenAddr, newState StateFromBytes[M, B]) chan error {
	conn, checksum, clock, err := SyncSocialState[M, B](cfg, peers, newState)
	if err != nil {
		finalize := make(chan error, 1)
		finalize <- fmt.Errorf("could not sync state: %v", err)
		return finalize
	}
	return launchNodeFromStateWithConnection[M, B](ctx, cfg, checksum, clock, conn)
}

type BlockSyncRequest struct {
	conn  *socket.SignedConnection
	state bool
	epoch uint64
}

// A client connecting must inform the server of its intention of the connection.
// It can either request block events subscruption (optionaly since a recent epoch)
// or full state syncronization.
func WaitForOutgoingSyncRequest(conn *socket.SignedConnection, syncRequest chan BlockSyncRequest) {
	data, err := conn.Read()
	if err != nil || len(data) < 1 {
		conn.Shutdown()
		return
	}
	switch data[0] {
	case messages.MsgProtocolSubscribe:
		if len(data) == 1 {
			syncRequest <- BlockSyncRequest{conn: conn, state: false, epoch: 0}
			return
		} else if len(data) == 9 {
			epoch, _ := util.ParseUint64(data, 1)
			syncRequest <- BlockSyncRequest{conn: conn, state: false, epoch: epoch}
			return
		}
	case messages.MsgProtocolStateSync:
		syncRequest <- BlockSyncRequest{conn: conn, state: true}
		return
	}
	conn.Shutdown()
}
