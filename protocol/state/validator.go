package state

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/protocol/actions"
)

const MaxEpochDifference = 100

// MutatingState is a validator for the breeze protocol. It contains the state
// and a mutation object. It keep tracks of collected fees.
type MutatingState struct {
	Epoch         uint64
	State         *State
	mutations     *Mutations
	FeesCollected uint64
}

// Incoraporate deposits the fees collected into the validator token and
// incorporate mutations into the state
func (m *MutatingState) Incorporate(validator crypto.Token) {
	validatorHash := crypto.HashToken(validator)
	if delta, ok := m.mutations.DeltaWallets[validatorHash]; ok {
		m.mutations.DeltaWallets[validatorHash] = delta + int(m.FeesCollected)
	} else {
		m.mutations.DeltaWallets[validatorHash] = int(m.FeesCollected)
	}
	m.State.IncorporateMutations(m.mutations)
}

// GetEpoch returns the epoch of the MutatingState
func (m *MutatingState) GetEpoch() uint64 {
	return m.Epoch
}

// Mutations returns the mutations of the MutatingState
func (m *MutatingState) Mutations() *Mutations {
	return m.mutations
}

// Validate validates the action (provided as a byte array) returns true if the
// action is valid, false otherwise.
func (c *MutatingState) Validate(data []byte) bool {
	action := actions.ParseAction(data)
	if action == nil {
		return false
	}
	//epoch := action.Epoch()
	//if (c.Epoch-epoch) > MaxEpochDifference || epoch > c.Epoch {
	//	fmt.Println("action incompatible epoch", c.Epoch, epoch)
	//	return false
	//}
	payments := action.Payments()
	if !c.CanPay(payments) {
		return false
	}
	c.TransferPayments(payments)
	return true
}

// Balance returns the balance of the account with the given hash
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

// CanPay returns true if the payments can be paid, false otherwise
func (b *MutatingState) CanPay(payments *actions.Payment) bool {
	for _, debit := range payments.Debit {
		existingBalance := b.Balance(debit.Account)
		if int(existingBalance) < int(debit.FungibleTokens) {
			return false
		}
	}
	return true
}

// CanWithdraw returns true if the account with the given hash can withdraw the
// given value, false otherwise
func (b *MutatingState) CanWithdraw(hash crypto.Hash, value uint64) bool {
	existingBalance := b.Balance(hash)
	return value < existingBalance
}

// Deposit Transfer resources from wallet to deposit. It does not check if
// wallet has enough resources.
func (b *MutatingState) Deposit(hash crypto.Hash, value uint64) {
	if old, ok := b.mutations.DeltaDeposits[hash]; ok {
		b.mutations.DeltaDeposits[hash] = old + int(value)
	} else {
		b.mutations.DeltaDeposits[hash] = int(value)
	}
	if balance, ok := b.mutations.DeltaWallets[hash]; ok {
		b.mutations.DeltaWallets[hash] = balance - int(value)
	} else {
		b.mutations.DeltaWallets[hash] = -int(value)
	}
}

// Withdraw Transfer resources from deposit to wallet. It does not check if
// the claim is valid.
func (b *MutatingState) Withdraw(hash crypto.Hash, value uint64) {
	if old, ok := b.mutations.DeltaDeposits[hash]; ok {
		b.mutations.DeltaDeposits[hash] = old - int(value)
	} else {
		b.mutations.DeltaDeposits[hash] = -int(value)
	}
	if balance, ok := b.mutations.DeltaWallets[hash]; ok {
		b.mutations.DeltaWallets[hash] = balance + int(value)
	} else {
		b.mutations.DeltaWallets[hash] = int(value)
	}
}

// Burn burns the given value from the deposit. It does not check if the claim
// is valid.
func (b *MutatingState) Burn(hash crypto.Hash, value uint64) {
	if old, ok := b.mutations.DeltaDeposits[hash]; ok {
		b.mutations.DeltaDeposits[hash] = old - int(value)
	} else {
		b.mutations.DeltaDeposits[hash] = -int(value)
	}
}

// TransferPayments transfer payments from one account to another. It does not
// check if the payments are valid.
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
