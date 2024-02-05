package blocks

import (
	"fmt"
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
	Complete
)

func (l *Server) WaitForRequests(conn *socket.SignedConnection) {
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
			return
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
			fmt.Println("new request...", len(data), data)
			req := ParseActionHistoryRequest(data)
			if req == nil {
				resp := []byte{ResponseError, data[1], data[2], data[3], data[4]}
				if err := conn.Send(resp); err != nil {
					slog.Info("error sending response", "err", err, "token", conn.Token.String(), "address", conn.Address)
					return
				}
			} else {
				go l.GetActions(req.Hash, req.Start, req.Seq, conn)
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

type actionHistoryRequest struct {
	Seq       uint32
	Start     uint64
	End       uint64
	Subscribe bool
	Hash      crypto.Hash
}

func ParseActionHistoryRequest(data []byte) *actionHistoryRequest {
	var req actionHistoryRequest
	if len(data) == 0 || data[0] != ActionHistoryRequest {
		return nil
	}
	position := 1
	req.Seq, position = util.ParseUint32(data, position)
	req.Start, position = util.ParseUint64(data, position)
	req.End, position = util.ParseUint64(data, position)
	req.Subscribe, position = util.ParseBool(data, position)
	req.Hash, position = util.ParseHash(data, position)
	if position != len(data) {
		fmt.Println("invalid action history request", len(data), position)
		return nil
	}
	fmt.Printf("%+v\n", req)
	return &req
}

func (l *Server) GetActions(hash crypto.Hash, epoch uint64, seq uint32, conn *socket.SignedConnection) {
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
			data = data[0:5]
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
	data[0] = Complete
	if err := conn.Send(data[0:5]); err != nil {
		conn.Shutdown()
	}
}

func (l *Server) GetBlocks(start, end uint64, seq uint32, conn *socket.SignedConnection) {
	if end == 0 {
		l.mu.Lock()
		end = l.provider.Epoch
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
