package actions

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

const (
	ITransfer byte = iota
	IDeposit
	IWithdraw
	IVoid
	IUnkown
)

const (
	NoProtocol uint32 = iota
	Breeze
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

type Action interface {
	Payments() *Payment
	Serialize() []byte
	Epoch() uint64
	Kind() byte
	FeePaid() uint64
	Tokens() []crypto.Token
}

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
		return NoProtocol
	}
	if action[1] == ITransfer || action[1] == IDeposit || action[1] == IWithdraw {
		return Breeze
	}
	if action[1] == IVoid {
		protocol, _ := util.ParseUint32(action, 10)
		return protocol
	}
	return NoProtocol
}
