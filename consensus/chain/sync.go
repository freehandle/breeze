package chain

import (
	"log/slog"

	"github.com/freehandle/breeze/socket"
)

func (c *Blockchain) SyncBlocksServer(conn *socket.CachedConnection, epoch uint64) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("sync blocks server panic", r)
		}
	}()
	c.mu.Lock()
	if len(c.RecentBlocks) > 0 && epoch < c.RecentBlocks[0].Header.Epoch {
		c.mu.Unlock()
		conn.Send(append([]byte{MsgSyncError}, []byte("node does not have information that old")...))
		conn.Close()
		conn.Live = false
		return
	}
	cacheCommit := make([]*CommitBlock, 0)
	cacheSealed := make([]*SealedBlock, 0)
	for _, block := range c.RecentBlocks {
		if block.Header.Epoch > epoch {
			cacheCommit = append(cacheCommit, block)
		}
	}
	for _, block := range c.SealedBlocks {
		if block.Header.Epoch > epoch {
			cacheSealed = append(cacheSealed, block)
		}
	}
	c.mu.Unlock()
	for _, block := range cacheCommit {
		conn.SendDirect(append([]byte{MsgBlockCommitted}, block.Serialize()...))
	}
	for _, block := range cacheSealed {
		conn.SendDirect(append([]byte{MsgBlockSealed}, block.Serialize()...))
	}
	conn.Ready()
}

func (c *BlockBuilder) SyncBlocksClient(nodeAddr string) chan bool {
	status := make(chan bool, 2)

	return status
}

func (c *Blockchain) SyncState(conn *socket.CachedConnection) {
	c.mu.Lock()
	wallet := c.Checksum.State.Wallets.HS.Bytes()
	deposits := c.Checksum.State.Deposits.HS.Bytes()
	c.mu.Unlock()
	if err := conn.SendDirect(append([]byte{MsgSyncStateWallets}, wallet...)); err != nil {
		slog.Error("sync state: could not send wallets", "err", err)
		conn.Close()
		return
	}
	if err := conn.SendDirect(append([]byte{MsgSyncStateDeposits}, deposits...)); err != nil {
		slog.Error("sync state: could not send wallets", "err", err)
		conn.Close()
		return
	}
}
