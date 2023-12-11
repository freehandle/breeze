package relay

import (
	"sync"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

// Admin messages type enum
const (
	MsgAddGateway byte = iota
	MsgRemoveGateway
	MsgAddBlocklistener
	MsgRemoveBlocklistener
	MsgUnkown = 255
)

// AdminMsgType returns the type of the admin message
func AdminMsgType(msg []byte) byte {
	if len(msg) < 9 || msg[8] > MsgRemoveBlocklistener {
		return MsgUnkown
	}
	return msg[8]
}

// ParseTokenMsg parses a message of the form [uint64][token] and returns the
// number and the token.
func ParseTokenMsg(msg []byte) (uint64, crypto.Token) {
	if len(msg) < 1 {
		return 0, crypto.ZeroToken
	}
	count, position := util.ParseUint64(msg, 0)
	token, _ := util.ParseToken(msg, position+1)
	return count, token
}

// Response returns a response message of the form [uint64][bool] where the
func Response(count uint64, ok bool) []byte {
	bytes := util.Uint64ToBytes(count)
	if ok {
		return append(bytes, 1)
	} else {
		return append(bytes, 0)
	}
}

type AdminConsole struct {
	mu       sync.Mutex
	count    uint64
	conn     *socket.SignedConnection
	messages map[uint64]chan []byte
}

func (n *AdminConsole) QueueMessage(msg []byte) chan []byte {
	n.mu.Lock()
	defer n.mu.Unlock()
	response := make(chan []byte, 2)
	n.count += 1
	msg = append(util.Uint64ToBytes(n.count), msg...)
	if err := n.conn.Send(msg); err != nil {
		response <- nil
		return response
	}
	n.messages[n.count] = response
	return response
}

func NewAdminConsole(conn *socket.SignedConnection) *AdminConsole {
	console := AdminConsole{
		mu:       sync.Mutex{},
		conn:     conn,
		messages: make(map[uint64]chan []byte),
	}
	go func() {
		for {
			msg, err := conn.Read()
			if len(msg) < 8 || err != nil {
				return
			}
			count, _ := util.ParseUint64(msg, 0)
			if reponse, ok := console.messages[count]; ok {
				reponse <- msg[8:]
				console.mu.Lock()
				delete(console.messages, count)
				console.mu.Unlock()
			}
		}
	}()
	return &console
}

func (n *AdminConsole) AddGateway(token crypto.Token) bool {
	msg := append([]byte{MsgAddGateway}, token[:]...)
	response := <-n.QueueMessage(msg)
	return len(response) > 0 && response[0] == 1
}

func (n *AdminConsole) RemoveGateway(token crypto.Token) bool {
	msg := append([]byte{MsgRemoveGateway}, token[:]...)
	response := <-n.QueueMessage(msg)
	return len(response) > 0 && response[0] == 1
}

func (n *AdminConsole) AddBlocklistener(token crypto.Token) bool {
	msg := append([]byte{MsgAddBlocklistener}, token[:]...)
	response := <-n.QueueMessage(msg)
	return len(response) > 0 && response[0] == 1
}

func (n *AdminConsole) RemoveBlocklistener(token crypto.Token) bool {
	msg := append([]byte{MsgRemoveBlocklistener}, token[:]...)
	response := <-n.QueueMessage(msg)
	return len(response) > 0 && response[0] == 1
}
