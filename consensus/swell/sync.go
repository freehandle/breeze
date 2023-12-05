package swell

import (
	"context"
	"errors"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/state"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

func FullSyncValidatorNode(ctx context.Context, config ValidatorConfig, syncAddress string, syncToken crypto.Token) error {

	conn, err := socket.Dial(config.hostname, syncAddress, config.credentials, syncToken)
	if err != nil {
		return err
	}
	bytes := []byte{chain.MsgSyncRequest}
	util.PutUint64(0, &bytes)
	util.PutBool(true, &bytes)
	conn.Send(bytes)

	clock := chain.ClockSyncronization{}

	msg, err := conn.Read()
	if err != nil {
		return err
	}
	if len(msg) < 8 || msg[0] != chain.MsgClockSync {
		return errors.New("invalid clock sync message")
	}
	position := 1
	clock.Epoch, position = util.ParseUint64(msg, position)
	clock.TimeStamp, _ = util.ParseTime(msg, position)

	checksum, err := syncChecksum(conn, config.walletPath)
	if err != nil {
		return err
	}

	node := &SwellNode{
		blockchain:  chain.BlockchainFromChecksumState(checksum, clock, config.credentials, config.swellConfig.NetworkHash, config.swellConfig.BlockInterval),
		actions:     config.actions,
		credentials: config.credentials,
		config:      config.swellConfig,
	}
	node.RunNonValidatingNode(ctx, conn, true)
	return nil
}

func syncChecksum(conn *socket.SignedConnection, walletPath string) (*chain.Checksum, error) {
	checksum := chain.Checksum{}

	msg, err := conn.Read()
	if err != nil {
		return nil, err
	}
	if len(msg) < 1 || msg[0] != chain.MsgSyncChecksum {
		return nil, errors.New("invalid sync epoch message")
	}
	position := 1
	checksum.Epoch, position = util.ParseUint64(msg, position)
	checksum.Hash, position = util.ParseHash(msg, position)
	checksum.LastBlockHash, position = util.ParseHash(msg, position)

	checksum.State = &state.State{
		Epoch: checksum.Epoch,
	}

	msg, err = conn.Read()
	if err != nil {
		return nil, err
	}
	if len(msg) < 1 || msg[0] != chain.MsgSyncStateWallets {
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
	if len(msg) < 1 || msg[0] != chain.MsgSyncStateDeposits {
		return nil, errors.New("invalid sync deposit message")
	}

	if walletPath != "" {
		checksum.State.Deposits = state.NewFileWalletStoreFromBytes(walletPath, "deposit", msg[1:])
	} else {
		checksum.State.Deposits = state.NewMemoryWalletStoreFromBytes("deposit", msg[1:])
	}
	return &checksum, nil
}
