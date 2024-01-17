package social

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

const StateSyncSize = 10e6

func getClockSync(conn *socket.SignedConnection) (chain.ClockSyncronization, bool) {
	clock := chain.ClockSyncronization{}
	msg, err := conn.Read()
	if err != nil || len(msg) < 17 || msg[0] != messages.MsgClockSync {
		return clock, false
	}
	position := 1
	clock.Epoch, position = util.ParseUint64(msg, position)
	clock.TimeStamp, position = util.ParseTime(msg, position)
	if clock.TimeStamp.Year() < 2024 && clock.TimeStamp.Year() > 2124 {
		return clock, false
	}
	return clock, true
}

func SyncSocialState[M Merger[M], B Blocker[M]](cfg Configuration, newState func([]byte) (Stateful[M, B], bool)) (*socket.SignedConnection, *Checksum[M, B], chain.ClockSyncronization, error) {
	var conn *socket.SignedConnection
	var err error
	for _, source := range cfg.TrustedProviders {
		addr := fmt.Sprintf("%v:%v", source.Addr, cfg.BlocksSourcePort)
		conn, err = socket.Dial(cfg.Hostname, addr, cfg.Credentials, source.Token)
		if err == nil {
			break
		}
	}
	if conn == nil {
		return nil, nil, chain.ClockSyncronization{}, errors.New("could not connect to any trusted providers")
	}
	// get clock synchronization from trusted provider
	clock, hasClock := getClockSync(conn)
	if !hasClock {
		conn.Shutdown()
		return nil, nil, clock, errors.New("sync connection error: could not get clock sync")
	}

	conn.Send([]byte{messages.MsgProtocolSyncReq})
	bytes := make([]byte, 0)
	var checksum Checksum[M, B]
	for {
		data, err := conn.Read()
		if err != nil {
			conn.Shutdown()
			return nil, nil, clock, fmt.Errorf("sync connection error: %v", err)
		}
		if len(data) == 0 {
			conn.Shutdown()
			return nil, nil, clock, errors.New("sync connection error: zero length data recieved")
		}
		if data[0] == messages.MsgProtocolStateSync {
			if len(data) == 1 {
				break
			}
			bytes = append(bytes, data[1:]...)
		} else if data[0] == messages.MsgProtocolChecksumSync {
			position := 1
			checksum.Epoch, position = util.ParseUint64(data, position)
			checksum.LastBlockHash, position = util.ParseHash(data, position)
			checksum.Hash, position = util.ParseHash(data, position)
			if position > len(data) {
				conn.Shutdown()
				return nil, nil, clock, errors.New("sync connection error: unexpected message")
			}
		} else {
			conn.Shutdown()
			return nil, nil, clock, errors.New("sync connection error: unexpected message")
		}
	}
	var ok bool
	if checksum.State, ok = newState(bytes); !ok {
		conn.Shutdown()
		return nil, nil, clock, errors.New("sync connection error: could not create state")
	}
	return conn, &checksum, clock, nil
}

func (s *SocialBlockChain[M, B]) StateSync(conn *socket.CachedConnection) {
	s.mu.Lock()
	checksum := s.Checksum
	syncRecentBlocks := make([]*SocialBlock, 0)
	for _, recent := range s.recentBlocks {
		if recent.Block.Epoch > checksum.Epoch {
			syncRecentBlocks = append(syncRecentBlocks, recent.Block)
		}
	}
	s.mu.Unlock()
	go func() {
		state := checksum.State.Serialize()
		// checksum sync
		bytes := []byte{messages.MsgProtocolChecksumSync}
		util.PutUint64(checksum.Epoch, &bytes)
		util.PutHash(checksum.LastBlockHash, &bytes)
		util.PutHash(checksum.Hash, &bytes)
		if err := conn.SendDirect(bytes); err != nil {
			slog.Info("sync connection error", "err", err)
			conn.Close()
			return
		}
		// state sync
		sent := 0
		for n := 0; n < len(state)/StateSyncSize; n++ {
			if err := conn.SendDirect(append([]byte{messages.MsgProtocolStateSync}, state[n*StateSyncSize:(n+1)*StateSyncSize]...)); err != nil {
				slog.Info("sync connection error", "err", err)
				conn.Close()
				return
			}
			sent += StateSyncSize
		}
		if err := conn.SendDirect(append([]byte{messages.MsgProtocolStateSync}, state[sent:]...)); err != nil {
			slog.Info("sync connection error", "err", err)
			conn.Close()
			return
		}
		// eof state sync
		if err := conn.SendDirect([]byte{messages.MsgProtocolStateSync}); err != nil {
			slog.Info("sync connection error", "err", err)
			conn.Close()
			return
		}
		for _, block := range syncRecentBlocks {
			if err := conn.SendDirect(append([]byte{messages.MsgProtocolBlock}, block.Serialize()...)); err != nil {
				slog.Info("sync connection error", "err", err)
				conn.Close()
				return
			}
		}
		conn.Ready()
	}()
}

func (s *SocialBlockChain[M, B]) SyncBlocks(conn *socket.CachedConnection, epoch uint64) {
	s.mu.Lock()
	blocks := make([]*SocialBlock, 0)
	for _, recent := range s.recentBlocks {
		if recent.Block.Epoch >= epoch {
			blocks = append(blocks, recent.Block)
		}
	}
	s.mu.Unlock()
	for _, block := range blocks {
		if err := conn.SendDirect(append([]byte{messages.MsgProtocolBlock}, block.Serialize()...)); err != nil {
			slog.Info("sync connection error", "err", err)
			conn.Close()
			return
		}
	}
	conn.Ready()
}
