package util

import (
	"fmt"
	"io"
	"os"

	"github.com/freehandle/breeze/crypto/scrypt"

	"github.com/freehandle/breeze/crypto"
)

type SecureVault struct {
	SecretKey crypto.PrivateKey
	Entries   [][]byte
	file      io.WriteCloser
	cipher    crypto.Cipher
}

func (s *SecureVault) NewEntry(data []byte) error {
	sealed := s.cipher.Seal(data)
	s.Entries = append(s.Entries, data)
	bytes := make([]byte, 0)
	PutByteArray(sealed, &bytes)
	if n, err := s.file.Write(bytes); n != len(bytes) || err != nil {
		return fmt.Errorf("could not write entry to secure vault file: %v", err)
	}
	return nil
}

func (s *SecureVault) Close() {
	s.file.Close()
}

func NewSecureVault(password []byte, fileName string) (*SecureVault, error) {
	file, err := os.Create(fileName)
	if err != nil {
		return nil, fmt.Errorf("could not create secure vault file: %v", err)
	}
	data := make([]byte, 0)
	salt, _ := crypto.RandomAsymetricKey()
	PutToken(salt, &data)
	cipherKey, err := scrypt.Key(password, salt[:], 32768, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("could not generate cipher key from password and salt: %v", err)
	}
	_, secret := crypto.RandomAsymetricKey()
	vault := SecureVault{
		SecretKey: secret,
		Entries:   make([][]byte, 0),
		file:      file,
		cipher:    crypto.CipherFromKey(cipherKey),
	}
	sealed := vault.cipher.Seal(secret[:])
	PutByteArray(sealed, &data)
	if n, err := file.Write(data); n != len(data) || err != nil {
		return nil, fmt.Errorf("could not write header to secure vault file: %v", err)
	}
	return &vault, nil
}

func OpenVaultFromPassword(password []byte, fileName string) (*SecureVault, error) {
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_RDWR, os.ModeAppend)
	if err != nil {
		return nil, fmt.Errorf("could not open Secret Vault: %v", err)
	}
	vault := SecureVault{
		Entries: make([][]byte, 0),
		file:    file,
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("could not read Secret Vault: %v", err)
	}
	position := 0
	var salt crypto.Token
	salt, position = ParseToken(data, position)
	key, err := scrypt.Key(password, salt[:], 32768, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("could not discover cipher key from password and salt: %v", err)
	}
	vault.cipher = crypto.CipherFromKey(key)

	items := make([][]byte, 0)
	for {
		var sealed []byte
		sealed, position = ParseByteArray(data, position)
		if position > len(data) {
			return nil, fmt.Errorf("valut file seems corrupted")
		}
		naked, err := vault.cipher.Open(sealed)
		if err != nil {
			return nil, fmt.Errorf("could not decrypt key: %v", err)
		}
		items = append(items, naked)
		if position == len(data) {
			break
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("vault file seems corrupted, could not read private key")
	}
	if !crypto.IsValidPrivateKey(items[0]) {
		return nil, fmt.Errorf("vault file seems corrupted, could not read valid private key")
	}
	copy(vault.SecretKey[:], items[0])
	if len(items) > 1 {
		vault.Entries = items[1:]
	}
	return &vault, nil
}
