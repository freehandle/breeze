// socket implements a signed uncencrypted TCP socket.
package socket

import (
	"errors"
	"fmt"
	"net"

	"github.com/freehandle/breeze/crypto"
)

var ErrMessageTooLarge = errors.New("message size cannot be larger than 65.536 bytes")
var ErrInvalidSignature = errors.New("signature is invalid")

type Message struct {
	Token crypto.Token
	Data  []byte
}

func DialTCP(laddr, raddr *net.TCPAddr, credentials crypto.PrivateKey, token crypto.Token) (*SignedConnection, error) {
	conn, err := net.DialTCP("tcp", laddr, raddr)
	if err != nil {
		return nil, err
	}
	return performClientHandShake(conn, credentials, token)
}

func Dial(address string, credentials crypto.PrivateKey, token crypto.Token) (*SignedConnection, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	return performClientHandShake(conn, credentials, token)

}

type SignedConnection struct {
	Token crypto.Token
	key   crypto.PrivateKey
	conn  net.Conn
}

func (s *SignedConnection) Send(msg []byte) error {
	if len(msg) == 0 {
		return nil
	}
	lengthWithSignature := len(msg) + crypto.SignatureSize
	if lengthWithSignature > 1<<32-1 {
		return ErrMessageTooLarge
	}
	msgToSend := []byte{byte(lengthWithSignature), byte(lengthWithSignature >> 8),
		byte(lengthWithSignature >> 16), byte(lengthWithSignature >> 24)}
	signature := s.key.Sign(msg)
	msgToSend = append(append(msgToSend, msg...), signature[:]...)
	if n, err := s.conn.Write(msgToSend); n != lengthWithSignature+4 {
		return err
	}
	return nil
}

func (s *SignedConnection) mustReadN(N int) ([]byte, error) {
	buffer := make([]byte, 0, N)
	for {
		msg := make([]byte, N-len(buffer))
		nBytes, err := s.conn.Read(msg)
		if err != nil {
			return nil, err
		}
		if nBytes > 0 {
			buffer = append(buffer, msg[:nBytes]...)
			if len(buffer) == N {
				return buffer, nil
			}
		}
	}
}

func (s *SignedConnection) readWithoutCheck() ([]byte, error) {
	lengthBytes, err := s.mustReadN(4)
	if err != nil {
		return nil, err
	}
	length := int(lengthBytes[0]) + (int(lengthBytes[1]) << 8) + (int(lengthBytes[2]) << 16) + (int(lengthBytes[3]) << 24)
	if length == 0 {
		return nil, nil
	}
	msg, err := s.mustReadN(length)
	if err != nil {
		return nil, err
	}
	if len(msg) != length {
		return nil, errors.New("unexpected error: message too short")
	}
	return msg, nil
}

func (s *SignedConnection) Read() ([]byte, error) {
	bytes, err := s.readWithoutCheck()
	if err != nil {
		return nil, err
	}
	if len(bytes) < crypto.SignatureSize {
		return nil, fmt.Errorf("message too short:%v", len(bytes))
	}
	msg := bytes[0 : len(bytes)-crypto.SignatureSize]
	var signature crypto.Signature
	copy(signature[:], bytes[len(bytes)-crypto.SignatureSize:])
	if !s.Token.Verify(msg, signature) {
		return nil, ErrInvalidSignature
	}
	return msg, nil
}

func (s *SignedConnection) Listen(newMessages chan Message, shutdown chan crypto.Token) {
	go func() {
		for {
			data, err := s.Read()
			if err != nil {
				shutdown <- s.Token
				return
			}
			newMessages <- Message{Token: s.Token, Data: data}
		}
	}()
}

func (s *SignedConnection) Shutdown() {
	s.conn.Close()
}
