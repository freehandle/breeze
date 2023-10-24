package state

import (
	"fmt"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
)

const MaxEpochDifference = 100

type MutatingState struct {
	Epoch         uint64
	State         *State
	mutations     *Mutations
	FeesCollected uint64
}

func (m *MutatingState) Incorporate(validator crypto.Token) {
	validatorHash := crypto.HashToken(validator)
	if delta, ok := m.mutations.DeltaWallets[validatorHash]; ok {
		m.mutations.DeltaWallets[validatorHash] = delta + int(m.FeesCollected)
	} else {
		m.mutations.DeltaWallets[validatorHash] = int(m.FeesCollected)
	}
	m.State.IncorporateMutations(m.mutations)
}

func (m *MutatingState) GetEpoch() uint64 {
	return m.Epoch
}

func (m *MutatingState) Mutations() *Mutations {
	return m.mutations
}

func (c *MutatingState) Validate(data []byte) bool {
	action := actions.ParseAction(data)
	if action == nil {
		fmt.Println("action is nil")
		return false
	}
	//epoch := action.Epoch()
	//if (c.Epoch-epoch) > MaxEpochDifference || epoch > c.Epoch {
	//	fmt.Println("action incompatible epoch", c.Epoch, epoch)
	//	return false
	//}
	payments := action.Payments()
	if !c.CanPay(payments) {
		fmt.Println("cant pay")
		return false
	}
	c.TransferPayments(payments)
	return true
}

func (c *MutatingState) Balance(hash crypto.Hash) uint64 {
	_, balance := c.State.Wallets.BalanceHash(hash)
	if c.mutations == nil {
		return balance
	}
	delta := c.mutations.DeltaBalance(hash)
	if delta < 0 {
		balance = balance - uint64(-delta)
	} else {
		balance = balance + uint64(delta)
	}
	return balance
}

func (b *MutatingState) CanPay(payments *actions.Payment) bool {
	for _, debit := range payments.Debit {
		existingBalance := b.Balance(debit.Account)
		if int(existingBalance) < int(debit.FungibleTokens) {
			return false
		}
	}
	return true
}

func (b *MutatingState) CanWithdraw(hash crypto.Hash, value uint64) bool {
	existingBalance := b.Balance(hash)
	return value < existingBalance
}

func (b *MutatingState) Deposit(hash crypto.Hash, value uint64) {
	if old, ok := b.mutations.DeltaDeposits[hash]; ok {
		b.mutations.DeltaDeposits[hash] = old + int(value)
		return
	}
	b.mutations.DeltaDeposits[hash] = int(value)
}

func (b *MutatingState) Withdraw(hash crypto.Hash, value uint64) {
	if old, ok := b.mutations.DeltaDeposits[hash]; ok {
		b.mutations.DeltaDeposits[hash] = old - int(value)
		return
	}
	b.mutations.DeltaDeposits[hash] = -int(value)
}

func (b *MutatingState) TransferPayments(payments *actions.Payment) {
	for _, debit := range payments.Debit {
		if delta, ok := b.mutations.DeltaWallets[debit.Account]; ok {
			b.mutations.DeltaWallets[debit.Account] = delta - int(debit.FungibleTokens)
		} else {
			b.mutations.DeltaWallets[debit.Account] = -int(debit.FungibleTokens)
		}
	}
	for _, credit := range payments.Credit {
		if delta, ok := b.mutations.DeltaWallets[credit.Account]; ok {
			b.mutations.DeltaWallets[credit.Account] = delta + int(credit.FungibleTokens)
		} else {
			b.mutations.DeltaWallets[credit.Account] = int(credit.FungibleTokens)
		}
	}
}
