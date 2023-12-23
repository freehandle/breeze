package swell

import (
	"context"
	"errors"
	"fmt"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/consensus/store"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/state"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

// FullSyncValidatorNode tries to gather information from a given validator to
// form a new non-validating node. This is used to bootstrap a new node from
// scratch.
func FullSyncValidatorNode(ctx context.Context, config ValidatorConfig, sync socket.TokenAddr) error {

	conn, err := socket.Dial(config.Hostname, sync.Addr, config.Credentials, sync.Token)
	if err != nil {
		return err
	}

	bytes := []byte{messages.MsgSyncRequest}
	util.PutUint64(0, &bytes)
	util.PutBool(true, &bytes)
	conn.Send(bytes)

	clock := chain.ClockSyncronization{}

	msg, err := conn.Read()
	if err != nil {
		return err
	}
	if len(msg) < 1 || msg[0] != messages.MsgCommittee {
		return errors.New("invalid committee message type")
	}
	order, validators := ParseCommitee(msg[1:])
	if len(order) == 0 || len(validators) == 0 {
		fmt.Println(order, validators)
		return errors.New("invalid committee message")
	}
	weights := make(map[crypto.Token]int)
	for _, token := range order {
		weights[token] += 1
	}
	committe := Committee{
		hostname:    config.Hostname,
		credentials: config.Credentials,
		order:       order,
		weights:     weights,
		validators:  validators,
	}

	msg, err = conn.Read()
	if err != nil {
		return err
	}
	if len(msg) < 8 || msg[0] != messages.MsgClockSync {
		return errors.New("invalid clock sync message")
	}
	position := 1
	clock.Epoch, position = util.ParseUint64(msg, position)
	clock.TimeStamp, _ = util.ParseTime(msg, position)

	checksum, err := syncChecksum(conn, config.WalletPath)
	if err != nil {
		return err
	}

	node := &SwellNode{
		blockchain:  chain.BlockchainFromChecksumState(checksum, clock, config.Credentials, config.SwellConfig.NetworkHash, config.SwellConfig.BlockInterval, config.SwellConfig.ChecksumWindow),
		actions:     store.NewActionStore(ctx, checksum.Epoch),
		credentials: config.Credentials,
		config:      config.SwellConfig,
		relay:       config.Relay,
		hostname:    config.Hostname,
	}
	RunActionsGateway(ctx, config.Relay.ActionGateway, node.actions)
	windowDuration := uint64(config.SwellConfig.ChecksumWindow)
	windowStart := windowDuration*(checksum.Epoch/windowDuration) + 1
	window := Window{
		ctx:         ctx,
		Start:       windowStart,
		End:         windowStart + windowDuration - 1,
		Committee:   &committe,
		Node:        node,
		newBlock:    make(chan BlockConsensusConfirmation),
		unpublished: make([]*chain.ChecksumStatement, 0),
		published:   make([]*chain.ChecksumStatement, 0),
	}
	RunNonValidatorNode(&window, conn, true)
	return nil
}

// syncCheksum is called by FullSyncValidatorNode to gather the checksum from
// the given connection. It returns a Checksum structure that will be used to
// build an instance of a swell node synchronized to the network.
func syncChecksum(conn *socket.SignedConnection, walletPath string) (*chain.Checksum, error) {
	checksum := chain.Checksum{}

	msg, err := conn.Read()
	if err != nil {
		return nil, err
	}
	if len(msg) < 1 || msg[0] != messages.MsgSyncChecksum {
		return nil, errors.New("invalid sync epoch message")
	}
	position := 1
	checksum.Epoch, position = util.ParseUint64(msg, position)
	checksum.Hash, position = util.ParseHash(msg, position)
	checksum.LastBlockHash, _ = util.ParseHash(msg, position)
	checksum.State = &state.State{
		Epoch: checksum.Epoch,
	}

	msg, err = conn.Read()
	if err != nil {
		return nil, err
	}
	if len(msg) < 1 || msg[0] != messages.MsgSyncStateWallets {
		return nil, errors.New("invalid sync wallet message")
	}
	if walletPath != "" {
		checksum.State.Wallets = state.NewFileWalletStoreFromBytes(walletPath, "wallet", msg[1:])
	} else {
		checksum.State.Wallets = state.NewMemoryWalletStoreFromBytes("wallet", msg[1:])
	}

	msg, err = conn.Read()
	if err != nil {
		return nil, err
	}
	if len(msg) < 1 || msg[0] != messages.MsgSyncStateDeposits {
		return nil, errors.New("invalid sync deposit message")
	}

	if walletPath != "" {
		checksum.State.Deposits = state.NewFileWalletStoreFromBytes(walletPath, "deposit", msg[1:])
	} else {
		checksum.State.Deposits = state.NewMemoryWalletStoreFromBytes("deposit", msg[1:])
	}

	stateHash := checksum.State.ChecksumHash()
	if !stateHash.Equal(checksum.Hash) {
		fmt.Println("deu ruim", crypto.EncodeHash(stateHash), crypto.EncodeHash(checksum.Hash))
		return nil, errors.New("invalid state hash")
	}
	return &checksum, nil
}
