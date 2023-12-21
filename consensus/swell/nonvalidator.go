package swell

type ChecksumWindowEpochs struct {
	WindowStart uint64
	WindowEnd   uint64
	Checksum    uint64
	Statement   uint64
}

func EpochsFromWindow(window, windowSize int) ChecksumWindowEpochs {
	return ChecksumWindowEpochs{
		// first window start with block 1
		WindowStart: uint64(window*windowSize + 1),
		WindowEnd:   uint64((window + 1) * windowSize),
		// checksum in the middle of the window
		Checksum: uint64(window*windowSize + window/2),
		// statement at 90% of the window: disclosure of naked checksums
		Statement: uint64((window+1)*windowSize - windowSize/10),
	}
}

// GetChecksumWindowEpochs returns the epochs for the next checksum window given
// the epoch of the last checksum event (the epoch when the state was cloned).
func GetChecksumWindowEpochs(lastChecksum uint64, window int) ChecksumWindowEpochs {
	windowCount := lastChecksum / uint64(window)
	return ChecksumWindowEpochs{
		// first window start with block 1
		WindowStart: windowCount*uint64(window) + 1,
		WindowEnd:   (windowCount + 1) * uint64(window),
		// checksum in the middle of the window
		Checksum: windowCount*uint64(window) + uint64(window/2),
		// statement at 90% of the window: disclosure of naked checksums
		Statement: (windowCount+1)*uint64(window) - uint64(window/10),
	}
}

// JoinCandidateNode lauches a validator pool of connections with other
// accredited validators. And runs the swell node by the ValidatingNode engine.
// Notice that at the same time more than one engine can be active on the same
// swell node. JoinCandidateNode is called befero the ending of the current
// checksum window, in order to give time for the proper network formation.
