package simple

import (
	"context"
	"fmt"
	"time"

	"github.com/freehandle/breeze/util"
	"github.com/freehandle/breeze/util/solo"
)

const chunkSize = 1024 * 1024 // 1 MB

type SimpleBlockWriter struct {
	writer *solo.Writer
}

func (s *SimpleBlockWriter) WriteBlock(block *SimpleBlock) error {
	data := make([]byte, 0)
	util.PutUint64(block.Epoch, &data)
	util.PutActionsArray(block.Actions, &data)
	_, err := s.writer.Write(data)
	return err
}

func OpenSimpleBlockWriter(path, name string, maxSize int64, output chan *SimpleBlock) (*SimpleBlockWriter, error) {
	chunkData := make(chan []byte, 1)
	fmt.Println("Opening simple block writer")
	writer, err := solo.NewWriter(path, name, maxSize, chunkSize, chunkData)
	if err != nil {
		return nil, err
	}
	fmt.Println("Opening chunk reader")
	reader := NewChunkBlockReader()
	for {
		chunk, ok := <-chunkData
		if !ok {
			break
		}
		fmt.Println("Incorporating chunk of size", len(chunk))
		blocks := reader.incorporate(chunk)
		for _, block := range blocks {
			//fmt.Println("Outputting block with epoch", block.Epoch, "and", len(block.Actions), "actions")
			output <- block
		}
	}
	close(output)
	if reader.bufferEpoch != 0 || len(reader.buffer) != 0 {
		return nil, fmt.Errorf("incomplete block data remaining in buffer")
	}
	return &SimpleBlockWriter{
		writer: writer,
	}, nil
}

func DissociateActions(ctx context.Context, block chan *SimpleBlock) chan []byte {
	actionChan := make(chan []byte, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(actionChan)
				return
			case b, ok := <-block:
				if !ok {
					close(actionChan)
					return
				}
				// first send the epoch signal
				blockEpochSignal := []byte{0}
				util.PutUint64(b.Epoch, &blockEpochSignal)
				actionChan <- blockEpochSignal
				//fmt.Println("Block epoch:", b.Epoch, "with", len(b.Actions), "actions")
				// then send all actions
				for _, action := range b.Actions {
					actionChan <- append([]byte{1}, action...)
				}
			}
		}
	}()
	return actionChan
}

func NewBlockReader(ctx context.Context, path, name string, interval time.Duration) chan *SimpleBlock {
	reader := solo.NewReader(path, name, chunkSize, interval)
	chunkChan := make(chan []byte, 1)
	blockChan := make(chan *SimpleBlock, 1)
	blockReader := NewChunkBlockReader()
	go func() {
		err := reader.Read(ctx, chunkChan)
		if err != nil {
			close(chunkChan)
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(blockChan)
				return
			case chunk, ok := <-chunkChan:
				if !ok {
					close(blockChan)
					return
				}
				blocks := blockReader.incorporate(chunk)
				for _, block := range blocks {
					blockChan <- block
				}
			}
		}
	}()
	return blockChan
}

type BlockChunkReader struct {
	bufferEpoch       uint64   // epoch of the data in buffer
	bufferActionCount int      // number of actions in buffer
	bufferActions     [][]byte // actions in buffer
	buffer            []byte   // incomplete data buffer
}

func NewChunkBlockReader() *BlockChunkReader {
	return &BlockChunkReader{
		bufferEpoch:       0,
		bufferActionCount: -1,
		bufferActions:     nil,
		buffer:            make([]byte, 0),
	}
}

func (s *BlockChunkReader) incorporate(chunk []byte) []*SimpleBlock {
	s.buffer = append(s.buffer, chunk...)
	blocks := make([]*SimpleBlock, 0)
	for {
		if len(s.buffer) == 0 {
			return blocks
		}
		// if there is no buffered epoch, try to read it
		if s.bufferEpoch == 0 {
			if len(s.buffer) < 8 {
				return blocks
			}
			// set epoch and remove from buffer
			s.bufferEpoch, _ = util.ParseUint64(s.buffer, 0)
			s.buffer = s.buffer[8:]
			s.bufferActionCount = -1
		}
		// if there is no buffered action count, try to read it
		if s.bufferActionCount == -1 {
			if len(s.buffer) < 4 {
				return blocks
			}
			actionCount, _ := util.ParseUint32(s.buffer, 0)
			s.bufferActionCount = int(actionCount)
			s.buffer = s.buffer[4:]
			s.bufferActions = make([][]byte, 0, s.bufferActionCount)
			// if there are no actions, we can finalize the block
			if s.bufferActionCount == 0 {
				block := &SimpleBlock{
					Epoch:   s.bufferEpoch,
					Actions: s.bufferActions,
				}
				blocks = append(blocks, block)
				// reset buffer state
				s.bufferEpoch = 0
				s.bufferActions = nil
				continue
			}
		}
		// try to read actions
		if len(s.buffer) < 2 {
			return blocks
		}
		length := int(s.buffer[0]) | int(s.buffer[1])<<8
		fmt.Println("Next action length:", length)
		if len(s.buffer) < 2+length {
			return blocks
		}
		s.bufferActions = append(s.bufferActions, s.buffer[2:2+length])
		s.buffer = s.buffer[2+length:]
		if len(s.bufferActions) == s.bufferActionCount {
			fmt.Println("Finalizing block with epoch", s.bufferEpoch, "and", s.bufferActionCount, "actions")
			block := &SimpleBlock{
				Epoch:   s.bufferEpoch,
				Actions: s.bufferActions,
			}
			blocks = append(blocks, block)
			// reset buffer state
			s.bufferEpoch = 0
			s.bufferActionCount = 0
			s.bufferActions = nil
		}
		fmt.Println("buuugfer", len(s.buffer))
	}
}
