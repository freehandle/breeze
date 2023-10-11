package chain

import "time"

func (c *Chain) MarkCheckpoint(done chan bool) {
	c.mu.Lock()
	c.Cloning = true
	go func() {
		epoch := c.LastCommitEpoch
		clonedState := c.CommitState.Clone()
		if clonedState == nil {
			done <- false
			return
		}
		c.Checksum = &Checksum{
			Epoch: epoch,
			State: clonedState,
			Hash:  clonedState.ChecksumHash(),
		}
		time.Sleep(5 * time.Second)
		c.Cloning = false
		done <- true
	}()
	c.mu.Unlock()
}
