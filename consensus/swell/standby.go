package swell

import (
	"context"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

type StandByNode struct {
	hostname   string
	Blockchain *chain.Blockchain
	connPool   *socket.Aggregator
	LastEvents *util.Await
}

type WindowWithWorker struct {
	window     *Window
	validators []socket.TokenAddr
	worker     chan *chain.SealedBlock
}

func RunReplicaNode(w *Window, conn *socket.SignedConnection) *StandByNode {
	if w == nil {
		slog.Error("RunReplicaNode: called with net window")
		return nil
	}
	var validators []socket.TokenAddr
	if committee := w.Committee; committee != nil {
		validators = w.Committee.validators
	}

	pool := socket.NewAgregator(w.ctx, w.Node.hostname, w.Node.credentials, conn)
	if pool == nil {
		slog.Error("RunReplicaNode: could not create aggregator")
		return nil
	}
	node := &StandByNode{
		hostname:   w.Node.hostname,
		Blockchain: w.Node.blockchain,
		connPool:   pool,
		LastEvents: util.NewAwait(w.ctx),
	}

	nextWindow := make(chan *WindowWithValidators)
	w.nextListener = nextWindow

	newCtx, cancelFunc := context.WithCancel(w.ctx)

	newSealed := ReadMessages(cancelFunc, pool)
	jobs := make(chan uint64)

	activeWindows := []*WindowWithWorker{
		{
			window:     w,
			validators: validators,
			worker:     RunReplicaWindow(newCtx, w, jobs),
		},
	}

	go func() {
		defer pool.Shutdown()
		canceled := w.ctx.Done()
		for {
			select {
			case <-canceled:
				slog.Info("RunReplicaNode: service terminated by context")
				return
			case next := <-nextWindow:
				_, err := node.connPool.AddOne(next.validators)
				if err != nil {
					slog.Warn("RunReplicaNode: could not add validator to pool", "err", err)
				}
				worker := &WindowWithWorker{
					window:     next.window,
					validators: next.validators,
					worker:     RunReplicaWindow(newCtx, next.window, jobs),
				}
				activeWindows = append(activeWindows, worker)
				slog.Info("RunReplicaNode: new window received", "start", next.window.Start, "end", next.window.End)
			case sealed := <-newSealed:
				epoch := sealed.Header.Epoch
				if !node.LastEvents.Call() {
					slog.Warn("RunReplicaNode: Await is closed.")
					return
				}
				found := false
				for _, w := range activeWindows {
					if epoch >= w.window.Start && epoch <= w.window.End {
						w.worker <- sealed
						found = true
						break
					}
				}
				if !found {
					slog.Warn("RunReplicaNode: sealed block received out of window", "epoch", epoch, "window start", w.Start, "window end", w.End)
				}
			case epoch := <-jobs:
				slog.Info("window job finished", "epoch", epoch)
				found := false
				for n, window := range activeWindows {
					if window.window.End == epoch {
						activeWindows = append(activeWindows[:n], activeWindows[n+1:]...)
						found = true
						break
					}
				}
				if !found {
					slog.Warn("window job finished but not found", "epoch", epoch)
				}
			}
		}
	}()
	return node
}

func RunReplicaWindow(ctx context.Context, w *Window, finished chan uint64) chan *chain.SealedBlock {
	sealed := make(chan *chain.SealedBlock)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sealedBlock := <-sealed:
				if sealedBlock == nil {
					slog.Warn("RunReplicaWindow: nil sealed block received")
				} else if epoch := sealedBlock.Header.Epoch; epoch < w.Start || epoch >= w.End {
					slog.Warn("RunReplicaWindow: sealed block received out of window", "epoch", epoch, "window start", w.Start, "window end", w.End)
				} else {
					w.AddSealedBlock(sealedBlock)
					if w.Finished() {
						finished <- w.End
						return
					}
				}
			}
		}
	}()
	return sealed
}
