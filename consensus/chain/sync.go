package chain

import (
	"fmt"

	"github.com/freehandle/breeze/socket"
)

func (c *Chain) SyncBlocksServer(conn *socket.CachedConnection, epoch uint64) {
	c.mu.Lock()
	if len(c.RecentBlocks) > 0 && epoch < c.RecentBlocks[0].Header.Epoch {
		c.mu.Unlock()
		fmt.Println("node does not have information that old")
		conn.Send(append([]byte{MsgSyncError}, []byte("node does not have information that old")...))
		conn.Close()
		conn.Live = false
		return
	}
	if epoch >= c.LiveBlock.Header.Epoch {
		fmt.Println("already sync", epoch, c.LiveBlock.Header.Epoch)
		conn.Ready()
		c.mu.Unlock()
		return
	}
	cacheCommit := make([]*CommitBlock, 0)
	cacheSealed := make([]*SealedBlock, 0)
	cloneLive := c.LiveBlock.Clone()
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
	fmt.Println("sending commit blocks", len(cacheCommit))
	for _, block := range cacheCommit {
		conn.SendDirect(append([]byte{MsgBlockCommitted}, block.Serialize()...))
	}
	fmt.Println("sending sealed blocks", len(cacheSealed))
	for _, block := range cacheSealed {
		conn.SendDirect(append([]byte{MsgBlockSealed}, block.Serialize()...))
	}
	fmt.Println("sending live block header")
	conn.SendDirect(append([]byte{MsgNewBlock}, cloneLive.Header.Serialize()...))
	fmt.Println("sending live block actions")
	for n := 0; n < cloneLive.Actions.Len(); n++ {
		conn.SendDirect(append([]byte{MsgAction}, cloneLive.Actions.Get(n)...))
	}
	fmt.Println("sending live block complete")
	conn.Ready()
}

func (c *Chain) SyncBlocksClient(nodeAddr string) chan bool {
	status := make(chan bool, 2)
	return status
}

func SyncState(conn *socket.CachedConnection) {
	// TODO
}
