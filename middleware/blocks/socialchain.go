package blocks

/*type SocialBlockProvider struct {
	mu         sync.Mutex
	recent     []*social.SocialBlock
	LastEvents *util.Await
	indexFn    func([]byte) []crypto.Hash
	keepN      int
}

// RecentAfter returns consecutive blocks after the given epoch. If there is a
// gap between blocks, the returned slice will be shorter than the number of
// blocks requested. If there is no consecutive block to epoch,
// RecentAfter will return an empty slice.
func (s *SocialBlockProvider) RecentAfter(epoch uint64) []*blockdb.IndexedBlock {
	s.mu.Lock()
	defer s.mu.Unlock()
	blocks := make([]*social.SocialBlock, 0)
	lastEpoch := epoch
	for _, block := range s.recent {
		if block.Epoch > epoch {
			if block.Epoch == lastEpoch+1 {
				blocks = append(blocks, block)
				lastEpoch = block.Epoch
			} else {
				break
			}
		}
	}
	indexed := make([]*blockdb.IndexedBlock, len(blocks))
	for n, block := range blocks {
		indexed[n] = &blockdb.IndexedBlock{
			Epoch: block.Epoch,
			Data:  block.Serialize(),
			Items: s.CommitToIndex(block),
		}
	}
	return indexed
}

func (s *SocialBlockProvider) Append(block *social.SocialBlock) bool {
	s.mu.Lock()
	if !s.LastEvents.Call() {
		s.mu.Unlock()
		return false
	}
	for n, recent := range s.recent {
		if block.Epoch > recent.Epoch {
			s.recent = append(append(s.recent[0:n], block), s.recent[n:]...)
			break
		}
	}
	if len(s.recent) > s.keepN {
		s.recent = s.recent[len(s.recent)-s.keepN:]
	}
	s.mu.Unlock()
	return true
}

func NewSocialBlockProvider(ctx context.Context, protocol uint32, indexFn func([]byte) []crypto.Hash, sources *socket.TrustedAggregator) {
	blocks := make(chan *social.SocialBlock)
	commits := make(chan *social.SocialBlockCommit)
	ctxC, cancel := context.WithCancel(ctx)
	social.SocialProtocolBlockListener(ctxC, protocol, sources, blocks, commits)

	provider := SocialBlockProvider{
		mu:         sync.Mutex{},
		recent:     make([]*social.SocialBlock, 0),
		LastEvents: util.NewAwait(ctx),
		indexFn:    indexFn,
		keepN:      100,
	}

	notCommit := make(map[uint64]*social.SocialBlock)
	go func() {
		defer cancel()
		for {
			done := ctx.Done()
			select {
			case <-done:
				return
			case block := <-blocks:
				if block.CommitSignature == crypto.ZeroSignature {
					notCommit[block.Epoch] = block
					continue
				} else if !provider.Append(block) {
					return
				}
			case commit := <-commits:
				if block, ok := notCommit[commit.Epoch]; ok {
					delete(notCommit, commit.Epoch)
					if !provider.Append(block) {
						return
					}
				}
			}
		}
	}()
}
*/
