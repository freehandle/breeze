package blocks

import (
	"context"
	"errors"
	"fmt"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

type BlockRequest struct {
	Start     uint64
	End       uint64
	Subscribe bool
	Blocks    chan []byte
}

type ActionRequest struct {
	Start     uint64
	End       uint64
	Subscribe bool
	Hash      crypto.Hash
	Actions   chan []byte
}

type BlocksClient struct {
	live    bool
	conn    *socket.SignedConnection
	blocks  chan BlockRequest
	actions chan ActionRequest
}

func (c *BlocksClient) RequestActions(start, end uint64, hash crypto.Hash) ([][]byte, error) {
	if !c.live {
		return nil, errors.New("connection is lost")
	}
	response := make(chan []byte)
	c.actions <- ActionRequest{
		Start:     start,
		End:       end,
		Hash:      hash,
		Subscribe: false,
		Actions:   response,
	}
	data := make([][]byte, 0)
	for {
		action, ok := <-response
		if !ok {
			return data, nil
		}
		if len(action) > 0 {
			data = append(data, action)
		}
	}
}

func (c *BlocksClient) SubscribeActionsAfter(start uint64, hash crypto.Hash) (chan []byte, error) {
	if !c.live {
		return nil, errors.New("connection is lost")
	}
	response := make(chan []byte)
	c.actions <- ActionRequest{
		Start:     start,
		Subscribe: true,
		Hash:      hash,
		Actions:   response,
	}
	return response, nil
}

func (c *BlocksClient) RequestBlocks(start, end uint64) ([]*chain.CommitBlock, error) {
	if !c.live {
		return nil, errors.New("connection is lost")
	}
	response := make(chan []byte)
	c.blocks <- BlockRequest{
		Start:     start,
		End:       end,
		Subscribe: false,
		Blocks:    response,
	}
	data := make([]*chain.CommitBlock, 0)
	for {
		blockBytes, ok := <-response
		if !ok {
			return data, nil
		}
		block := chain.ParseCommitBlock(blockBytes)
		if block != nil {
			data = append(data, block)
		}
	}
}

func (c *BlocksClient) SubscribeBlocksAtfer(start uint64) (chan *chain.CommitBlock, error) {
	if !c.live {
		return nil, errors.New("connection is lost")
	}
	response := make(chan []byte)
	c.blocks <- BlockRequest{
		Start:     start,
		Subscribe: true,
		Blocks:    response,
	}
	return pipeBytesToBlock(response), nil
}

func pipeBytesToBlock(in chan []byte) chan *chain.CommitBlock {
	out := make(chan *chain.CommitBlock)
	go func() {
		for {
			bytes, ok := <-in
			if !ok {
				close(out)
				return
			}
			block := chain.ParseCommitBlock(bytes)
			if block != nil {
				out <- block
			}
		}
	}()
	return out
}

func DialBlocksProvider(ctx context.Context, hostname, addr string, credentials crypto.PrivateKey, token crypto.Token) (*BlocksClient, error) {
	conn, err := socket.Dial(hostname, addr, credentials, token)
	if err != nil {
		return nil, fmt.Errorf("could not dial %s: %s", addr, err)
	}

	client := BlocksClient{
		live:    true,
		conn:    conn,
		blocks:  make(chan BlockRequest),
		actions: make(chan ActionRequest),
	}

	var seq uint32
	response := make(map[uint32]chan []byte)

	connData := make(chan []byte)

	go func() {
		defer func() {
			conn.Shutdown()
			client.live = false
			close(client.actions)
			close(client.blocks)
			for _, resp := range response {
				close(resp)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case req := <-client.blocks:
				msg := []byte{BlockHistoryRequest}
				util.PutUint32(seq, &msg)
				util.PutUint64(req.Start, &msg)
				util.PutUint64(req.End, &msg)
				util.PutBool(req.Subscribe, &msg)
				if err := conn.Send(msg); err != nil {
					return
				}
				response[seq] = req.Blocks
			case req := <-client.actions:
				msg := []byte{ActionHistoryRequest}
				util.PutUint32(seq, &msg)
				util.PutUint64(req.Start, &msg)
				util.PutUint64(req.End, &msg)
				util.PutBool(req.Subscribe, &msg)
				util.PutHash(req.Hash, &msg)
				if err := conn.Send(msg); err != nil {
					return
				}
				response[seq] = req.Actions
			case data, ok := <-connData:
				if !ok {
					return
				}
				if len(data) < 5 {
					continue
				}
				dataSeq, _ := util.ParseUint32(data, 1)
				if resp, ok := response[dataSeq]; ok {
					resp <- data
					// server should never sent a end of stream message in
					// subscription
					if len(data) == 5 {
						close(resp)
						delete(response, dataSeq)
					}
				}
			}
		}
	}()

	go func() {
		for {
			data, err := conn.Read()
			if err != nil {
				return
			}
			if len(data) == 0 {
				continue
			}
			if len(data) >= 5 {
				connData <- data
			} else if data[1] == Bye {
				close(connData)
				client.live = false
				// conn shutdown managed by the other go-routine
				return
			}
		}
	}()
	return &client, nil
}
