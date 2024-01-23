package config

import (
	"context"
	"fmt"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/crypto/dh"
	"github.com/freehandle/breeze/socket"
	"github.com/freehandle/breeze/util"
)

func WaitForRemoteKeysSync(ctx context.Context, tokens []crypto.Token, hostname string, port int) map[crypto.Token]crypto.PrivateKey {
	_, tempKey := crypto.RandomAsymetricKey()
	return waitForRemoteKeysSyncWithTempKey(ctx, tokens, tempKey, hostname, port)
}

func waitForRemoteKeysSyncWithTempKey(ctx context.Context, tokens []crypto.Token, tempKey crypto.PrivateKey, hostname string, port int) map[crypto.Token]crypto.PrivateKey {
	syncToken := tempKey.PublicKey()
	listener, err := socket.Listen(fmt.Sprintf("%s:%v", hostname, port))
	if err != nil {
		fmt.Printf("Failed to synchronize keys: could not bind to port: %v\n", err)
		return nil
	}
	defer listener.Close()
	fmt.Printf("Wait for synchronization of %d keys on port %d with ephemeral token %v\n", len(tokens), port, syncToken)
	withcancel, cancel := context.WithCancel(ctx)
	go func() {
		<-withcancel.Done()
		fmt.Println("closing listener")
		listener.Close()
	}()

	remaining := make(map[crypto.Token]struct{})
	for _, token := range tokens {
		remaining[token] = struct{}{}
	}
	synced := make(map[crypto.Token]crypto.PrivateKey)
	// accept any connection with any of the demanded tokens
	firewall := socket.NewValidConnections([]crypto.Token{}, true)
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("deu ruim")
			cancel()
			break
		}
		trusted, err := socket.PromoteConnection(conn, tempKey, firewall)
		if err != nil {
			fmt.Printf("admin connection rejected: %v\n", err)
			continue
		}
		keys := DiffieHellmanExchangeServer(trusted, tokens)
		if len(keys) > 0 {
			count := 0
			for _, key := range keys {
				if _, ok := remaining[key.PublicKey()]; ok {
					delete(remaining, key.PublicKey())
					synced[key.PublicKey()] = key
					count++
				}
			}
			if count > 0 {
				if len(remaining) == 0 {
					fmt.Println("Successfully synchronized all keys")
					return synced
				}
				fmt.Printf("received %d keys, %d remaining\n", len(synced), len(remaining))
			}
		}
	}
	fmt.Println("Failed to synchronize keys")
	cancel()
	return nil
}

func DiffieHellmanExchangeServer(conn *socket.SignedConnection, tokens []crypto.Token) []crypto.PrivateKey {
	if len(tokens) > 256 {
		tokens = tokens[:256]
	}
	ephPK, eph := dh.NewEphemeralKey()
	bytes := make([]byte, crypto.TokenSize)
	copy(bytes, eph[:])
	util.PutTokenArray(tokens, &bytes)
	if err := conn.Send(bytes); err != nil {
		return nil
	}
	response, err := conn.Read()
	if err != nil {
		return nil
	}
	var remote crypto.Token
	copy(remote[:], response[0:crypto.TokenSize])
	cipher := dh.ConsensusCipher(ephPK, remote)
	secrets, err := cipher.Open(response[crypto.TokenSize:])
	if err != nil {
		fmt.Println("failed to open", err)
		return nil
	}
	if len(secrets)%crypto.PrivateKeySize != 0 {
		return nil
	}
	secretKeys := make([]crypto.PrivateKey, len(secrets)/crypto.PrivateKeySize)
	for i := 0; i < len(secretKeys); i++ {
		copy(secretKeys[i][:], secrets[i*crypto.PrivateKeySize:(i+1)*crypto.PrivateKeySize])
	}
	validated := make([]crypto.PrivateKey, 0)
	for _, pk := range secretKeys {
		for _, token := range tokens {
			if pk.PublicKey().Equal(token) {
				validated = append(validated, pk)
				break
			}
		}
	}
	conn.Send([]byte{0, byte(len(validated))})

	return validated
}

func DiffieHellmanExchangeClient(conn *socket.SignedConnection, vault map[crypto.Token]crypto.PrivateKey) bool {
	defer conn.Shutdown()
	msg, err := conn.Read()
	if err != nil {
		return false
	}
	var remote crypto.Token
	var tokens []crypto.Token
	position := 0
	remote, position = util.ParseToken(msg, position)
	tokens, position = util.ParseTokenArray(msg, position)
	if position != len(msg) {

		return false
	}
	ephPK, eph := dh.NewEphemeralKey()
	cipher := dh.ConsensusCipher(ephPK, remote)
	epheralBytes := make([]byte, crypto.TokenSize)
	copy(epheralBytes, eph[:])

	secretKeys := make([]byte, 0)
	for _, token := range tokens {
		if secret, ok := vault[token]; ok {
			secretKeys = append(secretKeys, secret[:]...)
		}
	}
	if len(secretKeys) == 0 {
		conn.Shutdown()
		return false
	}
	secret := cipher.Seal(secretKeys)
	secret = append(epheralBytes, secret...)
	if err := conn.Send(secret); err != nil {
		conn.Shutdown()
		return false
	}
	if msg, err := conn.Read(); err != nil || len(msg) != 2 || msg[0] != 0 || msg[1] != byte(len(secretKeys)/crypto.PrivateKeySize) {
		return false
	}
	return true
}
