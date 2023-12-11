/*
Package actions implements the actions of the Breeze protocol.

Within Breeze thre are four types of actions:

1. Transfer: A transfer action is used to transfer tokens from one account to
one or more other accounts. A transfer action is signed by the sender account.
2. Deposit: A deposit action is used to deposit tokens as guarantee for
participating in the consensus.
3. Withdraw: A withdraw action is used to withdraw tokens from the consensus.
4. Void: A void action is general purpose action that can intends to use
the Breeze protocol for the basic purpose of an action gateway for more
specialized protocols.

actions package implements the serialization and deserialization of the
mentioned actions. And provides basic interface to sign actions and verify
signatures.

In Breeze wallet/token are used interchangeably. A wallet is nothing but a
public key associated to a private key in a Ed25519 elliptic curve cryptography.
*/
package actions

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

// Msg Kind >= IUnkown is reserved for future use
const (
	IVoid byte = iota
	ITransfer
	IDeposit
	IWithdraw
	IUnkown
)

type HashAction struct {
	Action Action
	Hash   crypto.Hash
}

type Wallet struct {
	Account        crypto.Hash
	FungibleTokens uint64
}

type Payment struct {
	Debit  []Wallet
	Credit []Wallet
}

// Interface common to all actions. Payments results in all deltas in wallets
// according to the instructions in the action. FeePaid is the fee to be paid
// by the validator that incorporates the action in a blockchain. Tokens is
// the list of breeze wallets that are affected in the action.
type Action interface {
	Payments() *Payment
	Serialize() []byte
	Epoch() uint64
	Kind() byte
	FeePaid() uint64
	Tokens() []crypto.Token
}

// NewPayment creates a new payment with a debit account and value.
func NewPayment(debitAcc crypto.Hash, value uint64) *Payment {
	return &Payment{
		Debit:  []Wallet{{debitAcc, value}},
		Credit: []Wallet{},
	}
}

func (p *Payment) NewCredit(account crypto.Hash, value uint64) {
	for _, credit := range p.Credit {
		if credit.Account.Equal(account) {
			credit.FungibleTokens += value
			return
		}
	}
	p.Credit = append(p.Credit, Wallet{Account: account, FungibleTokens: value})
}

func (p *Payment) NewDebit(account crypto.Hash, value uint64) {
	for _, debit := range p.Debit {
		if debit.Account.Equal(account) {
			debit.FungibleTokens += value
			return
		}
	}
	p.Debit = append(p.Debit, Wallet{Account: account, FungibleTokens: value})
}

func Kind(data []byte) byte {
	return data[1]
}

func ParseAction(data []byte) Action {
	if data[0] != 0 {
		return nil
	}
	switch data[1] {
	case ITransfer:
		return ParseTransfer(data)
	case IDeposit:
		return ParseDeposit(data)
	case IWithdraw:
		return ParseWithdraw(data)
	case IVoid:
		return ParseVoid(data)
	}
	return nil
}

func GetTokens(data []byte) []crypto.Token {
	action := ParseAction(data)
	if action == nil {
		return nil
	}
	return action.Tokens()
}

func GetEpochFromByteArray(inst []byte) uint64 {
	epoch, _ := util.ParseUint64(inst, 2)
	return epoch
}

func GetFeeFromBytes(action []byte) uint64 {
	if len(action) < crypto.SignatureSize+8 {
		return 0
	}
	fees, _ := util.ParseUint64(action, len(action)-crypto.SignatureSize-8-1)
	return fees
}

func Protocol(action []byte) uint32 {
	if len(action) < 86 || action[0] != 0 {
		return 0
	}
	if action[1] == ITransfer || action[1] == IDeposit || action[1] == IWithdraw {
		return 0
	}
	if action[1] == IVoid {
		protocol, _ := util.ParseUint32(action, 10)
		return protocol
	}
	return 0
}
