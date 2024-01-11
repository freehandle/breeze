package socket

import (
	"crypto/subtle"
	"errors"
	"net"

	"github.com/freehandle/breeze/crypto"
)

var errCouldNotVerify = errors.New("could not verify communication")

// Simple implementation of hasdshake for signed communication between nodes.
//
// The protocol uses a ValidateConnection interface wwhich checks if the caller
// token is an authorized one.
//
// The caller must know from the onset not only the IP and port number for the
// connection but also the connection token to check the identity of the server.
//
// After establishing connection, the caller send the called the following
// message: its token naked and a random nonce which the called must sign to
// prove its identity.
//
// The called checks if the proposed token is an authorized token. If so, it
// sends the caller the following message: its own token, a signature with its
// own key of the proposed nonce and a new nonce to be signed by the caller.
//
// The caller checks if the token is the one expected and verify the signature.
// It signs the proposed nonce and send it to the called.
//
// The called verifies the signature and if ok, the connection is ready to be
// used.

// read the first byte (n) and read subsequent n-bytes from connection
func readhs(conn net.Conn) ([]byte, error) {
	length := make([]byte, 1)
	if n, err := conn.Read(length); n != 1 {
		return nil, err
	}
	msg := make([]byte, length[0])
	if n, err := conn.Read(msg); n != int(length[0]) {
		return nil, err
	}
	return msg, nil
}

func writehs(conn net.Conn, msg []byte) error {
	if len(msg) > 256 {
		return errors.New("msg too large to send")
	}
	msgToSend := append([]byte{byte(len(msg))}, msg...)
	if n, err := conn.Write(msgToSend); n != len(msgToSend) {
		return err
	}
	return nil
}

func performClientHandShake(conn net.Conn, prvKey crypto.PrivateKey, remotePub crypto.Token) (*SignedConnection, error) {
	// send own public key and a random nonce to be signed by the remote server
	pubKey := prvKey.PublicKey()
	nonce := crypto.Nonce()
	msgToSend := append(pubKey[:], nonce...)
	writehs(conn, msgToSend)

	// receive remote token, signature of provided nonce and a new nonce to sign
	resp, err := readhs(conn)
	if err != nil {
		return nil, err
	}
	if len(resp) != crypto.TokenSize+crypto.NonceSize+crypto.SignatureSize {
		return nil, errCouldNotVerify
	}
	// test if t he copy matches with subtle
	remoteToken := resp[0:crypto.TokenSize]
	var remoteSignature crypto.Signature
	copy(remoteSignature[:], resp[crypto.TokenSize:crypto.TokenSize+crypto.SignatureSize])
	remoteNonce := resp[crypto.TokenSize+crypto.SignatureSize:]
	if subtle.ConstantTimeCompare(remoteToken, remotePub[:]) != 1 {
		return nil, errCouldNotVerify
	}
	if !remotePub.Verify(nonce, remoteSignature) {
		return nil, errCouldNotVerify
	}
	signature := prvKey.Sign(remoteNonce)
	if writehs(conn, signature[:]) != nil {
		return nil, errCouldNotVerify
	}
	return &SignedConnection{
		Token: remotePub,
		conn:  conn,
		key:   prvKey,
		Live:  true,
	}, nil
}

// PromoteConnection promotes a connection to a signed connection. It performs
// the handshake and returns a SignedConnection if the handshake is successful.
func PromoteConnection(conn net.Conn, prvKey crypto.PrivateKey, validator ValidateConnection) (*SignedConnection, error) {
	// read client token, and random nopnce
	resp, err := readhs(conn)
	if err != nil {
		return nil, err
	}
	if len(resp) != crypto.TokenSize+crypto.NonceSize {
		return nil, errCouldNotVerify
	}
	var clientToken crypto.Token
	copy(clientToken[:], resp[0:crypto.TokenSize])
	// check if public key is a member: TODO check if is a validator
	var remoteToken crypto.Token
	copy(remoteToken[:], resp[:crypto.TokenSize])
	ok := validator.ValidateConnection(remoteToken)
	if !<-ok {
		conn.Close()
		return nil, errCouldNotVerify
	}

	nonce := resp[crypto.TokenSize:]
	signature := prvKey.Sign(nonce)
	token := prvKey.PublicKey()
	newNonce := crypto.Nonce()

	msgToSend := append(append(token[:], signature[:]...), newNonce...)
	if err := writehs(conn, msgToSend); err != nil {
		return nil, err
	}

	// receive signature of proposed nonce from client
	resp, err = readhs(conn)
	if err != nil {
		return nil, err
	}
	// test if the copy matches with subtle
	if len(resp) != crypto.SignatureSize {
		return nil, errCouldNotVerify
	}
	var clientSignature crypto.Signature
	copy(clientSignature[:], resp)
	if !remoteToken.Verify(newNonce, clientSignature) {
		return nil, errCouldNotVerify
	}
	return &SignedConnection{
		Token: remoteToken,
		conn:  conn,
		key:   prvKey,
		Live:  false,
	}, nil
}
