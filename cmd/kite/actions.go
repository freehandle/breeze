package main

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
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
	fmt.Println("dialing")
	conn, epoch, err := safe.dialGateway()
	if err != nil {
		fmt.Print(err)
		return err
	}
	fmt.Println("dialing ok")

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
	fmt.Println(transfer)
	err = conn.Send(transfer.Serialize())
	fmt.Println("ent")
	if err != nil {
		return fmt.Errorf("error sending transfer to gateway: %s", err)
	}
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
	err = conn.Send(msg)
	if err != nil {
		return fmt.Errorf("error sending transfer to gateway: %s", err)
	}
	return nil
}

type BalanceCommand struct {
	Account string
}

func (c *BalanceCommand) Execute(vault *Kite) error {
	return nil
}
