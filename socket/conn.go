// socket implements a signed uncencrypted TCP socket.
package socket

import (
	"errors"
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
	lengthWithSignature := len(msg) + crypto.SignatureSize
	if lengthWithSignature > 1<<40-1 {
		return ErrMessageTooLarge
	}
	msgToSend := []byte{byte(lengthWithSignature), byte(lengthWithSignature >> 8),
		byte(lengthWithSignature >> 16), byte(lengthWithSignature >> 24), byte(lengthWithSignature >> 32)}
	signature := s.key.Sign(msg)
	msgToSend = append(append(msgToSend, msg...), signature[:]...)
	if n, err := s.conn.Write(msgToSend); n != lengthWithSignature+4 {
		return err
	}
	return nil
}

func (s *SignedConnection) readWithoutCheck() ([]byte, error) {
	lengthBytes := make([]byte, 5)
	if n, err := s.conn.Read(lengthBytes); n != 5 {
		return nil, err
	}
	lenght := int(lengthBytes[0]) + (int(lengthBytes[1]) << 8) + (int(lengthBytes[2]) << 16) + (int(lengthBytes[3]) << 24) + (int(lengthBytes[4]) << 32)
	msg := make([]byte, lenght)
	if n, err := s.conn.Read(msg); n != int(lenght) {
		return nil, err
	}
	return msg, nil
}

func (s *SignedConnection) Read() ([]byte, error) {
	bytes, err := s.readWithoutCheck()
	if err != nil {
		return nil, err
	}
	if len(bytes) < crypto.SignatureSize {
		return nil, errors.New("message too short")
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
