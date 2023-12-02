package state

import (
	"fmt"
	"log/slog"

	"github.com/freehandle/breeze/crypto"
)

type State struct {
	Epoch    uint64
	Wallets  *Wallet // Available tokens per hash of crypto key
	Deposits *Wallet // Available stakes per hash of crypto key
}

func (s *State) NewMutations() *Mutations {
	return NewMutations(s.Epoch + 1)
}

func (s *State) Validator(mutations *Mutations, epoch uint64) *MutatingState {
	return &MutatingState{
		State:     s,
		mutations: mutations,
	}
}

func (s *State) Shutdown() {
	s.Wallets.Close()
}

func NewGenesisState() (*State, crypto.PrivateKey) {
	pubKey, prvKey := crypto.RandomAsymetricKey()
	state := NewGenesisStateWithToken(pubKey, "")
	return state, prvKey
}

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

func (s *State) Clone() *State {
	wallets := &Wallet{HS: s.Wallets.HS.Clone()}
	deposits := &Wallet{HS: s.Deposits.HS.Clone()}
	return &State{
		Epoch:    s.Epoch,
		Wallets:  wallets,
		Deposits: deposits,
	}
}

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

func (s *State) ChecksumHash() crypto.Hash {
	walletHash := s.Wallets.HS.Hash(crypto.Hasher)
	depositHash := s.Deposits.HS.Hash(crypto.Hasher)
	return crypto.Hasher(append(walletHash[:], depositHash[:]...))
}
