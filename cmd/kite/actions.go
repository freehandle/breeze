package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/freehandle/breeze/consensus/messages"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
	"github.com/freehandle/breeze/socket"
)

type TransferCommand struct {
	From    string
	To      string
	Ammount string
	Fee     string
	Reason  string
}

func (t *TransferCommand) Execute(safe *Kite) error {
	tokenFrom := crypto.TokenFromString(t.From)
	if tokenFrom == crypto.ZeroToken {
		return errors.New("invalid from account")
	}
	secret := safe.findSecret(tokenFrom)
	if secret == crypto.ZeroPrivateKey {
		return errors.New("cannot find secret of from account")
	}
	tokenTo := crypto.TokenFromString(t.To)
	if tokenTo == crypto.ZeroToken {
		return errors.New("invalid to account")
	}
	amount, err := strconv.Atoi(t.Ammount)
	if err != nil || amount <= 0 {
		return errors.New("invalid amount")
	}
	fee, err := strconv.Atoi(t.Fee)
	if err != nil || fee < 0 {
		return errors.New("invalid fee amount")
	}
	conn, epoch, err := safe.dialGateway()
	if err != nil {
		fmt.Print(err)
		return err
	}

	transfer := actions.Transfer{
		TimeStamp: epoch, // TODO
		From:      tokenFrom,
		To: []crypto.TokenValue{
			{Token: tokenTo, Value: uint64(amount)},
		},
		Reason: t.Reason,
		Fee:    uint64(fee),
	}
	transfer.Sign(secret)
	err = SendAndConfirm(conn, transfer.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func SendAndConfirm(conn *socket.SignedConnection, msg []byte) error {
	if err := conn.Send(append([]byte{messages.MsgAction}, msg...)); err != nil {
		return fmt.Errorf("error sending transfer to gateway: %s", err)
	}
	resp, err := conn.Read()
	if err != nil {
		return fmt.Errorf("error receiving response from gateway: %s", err)
	}
	if len(resp) == 0 || resp[0] != messages.MsgActionForward {
		return errors.New("action rejected by gateway")
	}
	resp, err = conn.Read()
	if err != nil {
		return fmt.Errorf("error receiving response from gateway: %s", err)
	}
	if resp[0] != messages.MsgActionSealed {
		return errors.New("action rejected by gateway")
	}
	msgHash := crypto.Hasher(msg)
	hash, epoch, sealHash := messages.ParseSealedAction(resp)
	if epoch == 0 || !hash.Equal(msgHash) {
		return errors.New("invalid sealed action")
	}
	fmt.Printf("action %v, incorporated into sealed block epoch %v with hash %v\n", hash, epoch, sealHash)
	return nil
}

type StakeCommand struct {
	Account string
	Ammount string
	Fee     string
	Deposit bool
}

func (s *StakeCommand) Execute(safe *Kite) error {
	tokenFrom := crypto.TokenFromString(s.Account)
	if tokenFrom == crypto.ZeroToken {
		return errors.New("invalid from account")
	}
	secret := safe.findSecret(tokenFrom)
	if secret == crypto.ZeroPrivateKey {
		return errors.New("cannot find secret of from account")
	}
	amount, err := strconv.Atoi(s.Ammount)
	if err != nil || amount <= 0 {
		return errors.New("invalid amount")
	}
	fee, err := strconv.Atoi(s.Fee)
	if err != nil || fee < 0 {
		return errors.New("invalid fee amount")
	}
	conn, epoch, err := safe.dialGateway()
	if err != nil {
		return err
	}
	var msg []byte
	if s.Deposit {
		deposit := actions.Deposit{
			TimeStamp: epoch, // TODO
			Token:     tokenFrom,
			Value:     uint64(amount),
			Fee:       uint64(fee),
		}
		deposit.Sign(secret)
		msg = deposit.Serialize()
	} else {
		withdraw := actions.Withdraw{
			TimeStamp: epoch, // TODO
			Token:     tokenFrom,
			Value:     uint64(amount),
			Fee:       uint64(fee),
		}
		withdraw.Sign(secret)
		msg = withdraw.Serialize()
	}
	err = SendAndConfirm(conn, msg)
	if err != nil {
		return err
	}
	return nil
}

type BalanceCommand struct {
	Account string
}

func (c *BalanceCommand) Execute(vault *Kite) error {
	return nil
}

type ListCommand struct {
	Token string
	Epoch string
}

func (c *ListCommand) Execute(vault *Kite) error {
	token := crypto.TokenFromString(c.Token)
	if token == crypto.ZeroToken {
		return errors.New("invalid token")
	}
	epoch := 0
	var err error
	if c.Epoch != "" {
		epoch, err := strconv.Atoi(c.Epoch)
		if err != nil || epoch < 0 {
			return errors.New("invalid epoch")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider, err := vault.dialBlockDatabase(ctx)
	if err != nil {

		return err
	}
	actionList, err := provider.RequestActions(uint64(epoch), 0, crypto.HashToken(token))
	if err != nil {
		return err
	}
	for _, bytes := range actionList {
		action := actions.ParseAction(bytes)
		if action != nil {
			fmt.Println(action.JSON())
		}
	}
	return nil
}
