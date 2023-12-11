package state

import (
	"fmt"
	"log/slog"

	"github.com/freehandle/breeze/crypto"
)

// State is the state of the blockchain. It contains the epoch, the wallets and
// the deposits.
type State struct {
	Epoch    uint64
	Wallets  *Wallet // Available tokens per hash of crypto key
	Deposits *Wallet // Available stakes per hash of crypto key
}

// NewMutations creates a new mutation object with the following epoch.
func (s *State) NewMutations() *Mutations {
	return NewMutations(s.Epoch + 1)
}

// Validator combines a state and a mutation into a mutating state for epoch.
func (s *State) Validator(mutations *Mutations, epoch uint64) *MutatingState {
	return &MutatingState{
		State:     s,
		mutations: mutations,
	}
}

// Shutdown graciously closes the state data stores.
func (s *State) Shutdown() {
	s.Wallets.Close()
	s.Deposits.Close()
}

// NewGenesisState creates a new genesis state minting funguble tokens to a
// random token. Returns the state and the associated private key.
func NewGenesisState() (*State, crypto.PrivateKey) {
	pubKey, prvKey := crypto.RandomAsymetricKey()
	state := NewGenesisStateWithToken(pubKey, "")
	return state, prvKey
}

// NewGenesisStateWithToken creates a new genesis state minting fungible tokens
// to a given address. Returns the state.
func NewGenesisStateWithToken(token crypto.Token, filePath string) *State {
	state := State{Epoch: 0}
	if filePath == "" {
		if wallet := NewMemoryWalletStore("wallet", 8); wallet != nil {
			state.Wallets = wallet
		} else {
			slog.Error("NewGenesisStateWithToken: could not create memory wallet")
			return nil
		}
		if deposit := NewMemoryWalletStore("deposit", 8); deposit != nil {
			state.Deposits = deposit
		} else {
			slog.Error("NewGenesisStateWithToken: could not create memory deposit")
			return nil
		}
	} else {
		if wallet := NewFileWalletStore(fmt.Sprintf("%vwallet.dat", filePath), "wallet", 8); wallet != nil {
			state.Wallets = wallet
		} else {
			slog.Error("NewGenesisStateWithToken: could not create file wallet")
			return nil
		}
		if deposit := NewFileWalletStore(fmt.Sprintf("%vdeposit.dat", filePath), "deposit", 8); deposit != nil {
			state.Deposits = deposit
		} else {
			slog.Error("NewGenesisStateWithToken: could not create file deposit")
			return nil
		}
	}
	if !state.Wallets.Credit(token, 1e9) {
		slog.Error("NewGenesisStateWithToken: could not credit wallet")
		return nil
	}
	if !state.Deposits.Credit(token, 1e9) {
		slog.Error("NewGenesisStateWithToken: could not credit deposit")
		return nil
	}
	return &state
}

// IncorporateMutations applies the mutations to the state.
func (s *State) IncorporateMutations(m *Mutations) {
	for hash, delta := range m.DeltaWallets {
		if delta > 0 {
			s.Wallets.CreditHash(hash, uint64(delta))
		} else if delta < 0 {
			s.Wallets.DebitHash(hash, uint64(-delta))
		}
	}
	for hash, delta := range m.DeltaDeposits {
		if delta > 0 {
			s.Deposits.CreditHash(hash, uint64(delta))
		} else if delta < 0 {
			s.Deposits.DebitHash(hash, uint64(-delta))
		}
	}
}

// Clone creates a copy of the state by cloning the underlying papirus hashtable
// stores.
func (s *State) Clone() *State {
	wallets := &Wallet{HS: s.Wallets.HS.Clone()}
	deposits := &Wallet{HS: s.Deposits.HS.Clone()}
	return &State{
		Epoch:    s.Epoch,
		Wallets:  wallets,
		Deposits: deposits,
	}
}

// CloneAsync starts a jobe to cloning the underlying hashtable stores. Returns
// a channel to a state object.
func (s *State) CloneAsync() chan *State {
	output := make(chan *State)
	wallets := s.Wallets.HS.CloneAsync()
	deposits := s.Wallets.HS.CloneAsync()
	newState := &State{
		Epoch: s.Epoch,
	}
	go func() {
		count := 0
		for {
			select {
			case wallet := <-wallets:
				count += 1
				newState.Wallets = &Wallet{HS: wallet}
			case deposit := <-deposits:
				count += 1
				newState.Deposits = &Wallet{HS: deposit}
			}
			if count == 2 {
				output <- newState
				return
			}
		}
	}()
	return output
}

// ChecksumHash returns the hash of the checksum of the state.
func (s *State) ChecksumHash() crypto.Hash {
	walletHash := s.Wallets.HS.Hash(crypto.Hasher)
	depositHash := s.Deposits.HS.Hash(crypto.Hasher)
	return crypto.Hasher(append(walletHash[:], depositHash[:]...))
}
