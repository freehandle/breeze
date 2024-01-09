package blocks

import (
	"log/slog"

	"github.com/freehandle/breeze/crypto"

	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

const (
	BlockHistoryRequest byte = iota + 1
	ActionHistoryRequest
	Bye
	ResponseError
	OutOfOrderError
	ActionHistoryResponse
	BlockHistoryResponse
)

func (l *ListenerNode) WaitForRequests(conn *socket.SignedConnection) {
	defer func() {
		l.mu.Lock()
		for n, poolConn := range l.live {
			if poolConn == conn {
				l.live = append(l.live[:n], l.live[n+1:]...)
				break
			}
		}
		l.mu.Unlock()
		conn.Shutdown()
	}()
	seq := uint32(0)
	for {
		data, err := conn.Read()
		if err != nil {
			slog.Info("connection closed", "token", conn.Token.String(), "address", conn.Address, "err", err)
		}
		if len(data) < 5 {
			continue
		}
		dataSeq, _ := util.ParseUint32(data, 1)
		if dataSeq != seq {
			if err := conn.Send([]byte{OutOfOrderError}); err != nil {
				return
			}
			continue
		}
		switch data[0] {
		case BlockHistoryRequest:
			if len(data) != 1+8+8 {
				if err := conn.Send([]byte{ResponseError}); err != nil {
					return
				}
				continue
			}

		case ActionHistoryRequest:
			if len(data) != 1+32 {
				if err := conn.Send([]byte{ResponseError}); err != nil {
					return
				}
				continue
			}
		}
		seq += 1
	}

}

func ParseHistoryRequest(data []byte) (uint64, uint64) {
	var start, end uint64
	start, _ = util.ParseUint64(data, 1)
	end, _ = util.ParseUint64(data, 9)
	return start, end
}

func ParseActionHistoryRequest(data []byte) crypto.Hash {
	var hash crypto.Hash
	hash, _ = util.ParseHash(data, 9)
	return hash
}

func (l *ListenerNode) GetActions(hash crypto.Hash, epoch uint64, seq uint32, conn *socket.SignedConnection) {
	actions := l.db.Find(hash, epoch)
	data := []byte{ActionHistoryResponse}
	util.PutUint32(seq, &data)
	// send actions in packets of 10000 messages each time
	for {
		if len(actions) > 1000 {
			util.PutActionsArray(actions[0:1000], &data)
			if err := conn.Send(data); err != nil {
				conn.Shutdown()
				return
			}
			data = data[0:9]
			actions = actions[1000:]
		} else {
			util.PutActionsArray(actions, &data)
			if err := conn.Send(data); err != nil {
				conn.Shutdown()
			}
			break
		}
	}
	// an empty action to indicate end of transmission
	if err := conn.Send(data[0:9]); err != nil {
		conn.Shutdown()
	}
}

func (l *ListenerNode) GetBlocks(start, end uint64, seq uint32, conn *socket.SignedConnection) {
	if end == 0 {
		l.mu.Lock()
		end = l.LastCommitEpoch
		isNew := true
		for _, subscriber := range l.subscribers {
			if subscriber == conn {
				isNew = false
				break
			}
		}
		if isNew {
			l.subscribers = append(l.subscribers, conn)
		}
		l.mu.Unlock()
	}
	if start > end {
		slog.Warn("invalid block history request", "start", start, "end", end)
		start, end = end, start
	}
	data := []byte{BlockHistoryResponse}
	util.PutUint32(seq, &data)
	for epoch := start; epoch <= end; epoch++ {
		block := l.db.Retrieve(int64(epoch), 0)
		if block == nil {
			continue
		}
		util.PutLargeByteArray(block, &data)
		if len(data) > 100e6 {
			if err := conn.Send(data); err != nil {
				conn.Shutdown()
			}
			data = data[0:9]
		}
	}
	// an empty block to indicate end of transmission
	if err := conn.Send(data[0:9]); err != nil {
		conn.Shutdown()
	}
}
