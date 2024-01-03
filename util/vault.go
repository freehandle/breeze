package util

import (
	"io"
	"log"
	"os"
	"sync"

	"github.com/freehandle/breeze/crypto/scrypt"

	"crypto/rand"

	"github.com/freehandle/breeze/crypto"
)

const (
	TypePrivateKey byte = iota
	TypeWalletPrivateKey
	TypeStageSecrets
)

type StageSecrets struct {
	Ownership  crypto.PrivateKey
	Moderation crypto.PrivateKey
	Submission crypto.PrivateKey
	CipherKey  []byte
}

type SecureVault struct {
	mu        sync.Mutex
	SecretKey crypto.PrivateKey
	Secrets   map[crypto.Token]crypto.PrivateKey
	file      io.WriteCloser
	cipher    crypto.Cipher
}

func (s *SecureVault) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file.Close()
}

func (vault *SecureVault) GenerateNewKey() (crypto.Token, crypto.PrivateKey) {
	vault.mu.Lock()
	defer vault.mu.Unlock()
	token, newKey := crypto.RandomAsymetricKey()
	sealed := vault.cipher.Seal(newKey[:])
	withLen := []byte{byte(len(sealed))}
	withLen = append(withLen, sealed...)
	if n, err := vault.file.Write(withLen); n != len(withLen) || err != nil {
		// TODO: this is serious problem
		log.Fatalf("secret vault is possibly compromissed: %v\n", err)
	}
	vault.Secrets[token] = newKey
	return token, newKey
}

func NewSecureVault(password []byte, fileName string) *SecureVault {
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("could not create secure vault file: %v\n", err)
	}

	salt := make([]byte, 32)
	rand.Read(salt)
	cipherKey, err := scrypt.Key(password, salt, 32768, 8, 1, 32)
	if err != nil {
		log.Fatalf("could not generate cipher key from password and salt: %v\n", err)
	}
	_, secret := crypto.RandomAsymetricKey()
	vault := SecureVault{
		SecretKey: secret,
		Secrets:   make(map[crypto.Token]crypto.PrivateKey),
		file:      file,
		cipher:    crypto.CipherFromKey(cipherKey),
	}
	if n, err := file.Write(salt); n != len(salt) || err != nil {
		log.Fatalf("could not write salto to secure vault file: %v\n", err)
	}
	sealed := vault.cipher.Seal(secret[:])
	if n, err := file.Write(append([]byte{byte(len(sealed))}, sealed...)); n != len(sealed)+1 || err != nil {
		log.Fatalf("could not write to secure vault file: %v\n", err)
	}
	return &vault
}

func OpenVaultFromPassword(password []byte, fileName string) *SecureVault {
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_RDWR, os.ModeAppend)
	if err != nil {
		log.Fatalf("Could not open Secret Vault: %v\n", err)
	}

	vault := SecureVault{
		Secrets: make(map[crypto.Token]crypto.PrivateKey),
		file:    file,
	}

	salt := make([]byte, 32)
	if n, err := io.ReadFull(file, salt); n != 32 {
		if err == io.EOF {
			log.Fatal("valut file seems corrupted. Could not read private key.")
		} else {
			log.Fatalf("valut file seems corrupted. Could not read salt: %v\n", err)
		}
	} else {
		key, err := scrypt.Key(password, salt, 32768, 8, 1, 32)
		if err != nil {
			log.Fatalf("could not discover cipher key from password and salt: %v\n", err)
		}
		vault.cipher = crypto.CipherFromKey(key)
	}

	first := true // first key is the private key for the wallet app itself.
	size := make([]byte, 1)
	for {
		if n, err := io.ReadFull(file, size); n == 1 {
			keyBytes := make([]byte, int(size[0]))
			if n, err := io.ReadFull(file, keyBytes); n == int(size[0]) {
				if naked, err := vault.cipher.Open(keyBytes); err != nil {
					log.Fatalf("could not decrypt key: %v\n", err)
				} else {
					var key crypto.PrivateKey
					copy(key[:], naked)
					if first {
						vault.SecretKey = key
						first = false
					}
					vault.Secrets[key.PublicKey()] = key
				}
			} else {
				log.Fatalf("could not parse key: %v, %v\n", err, size[0])
			}
			if err == io.EOF {
				break
			}
		} else {
			if err == io.EOF {
				break
			}
		}

	}
	return &vault
}
