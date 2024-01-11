// socket implements a signed uncencrypted TCP socket.
package socket

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/freehandle/breeze/crypto"
)

var ErrMessageTooLarge = errors.New("message size cannot be larger than 65.536 bytes")
var ErrInvalidSignature = errors.New("signature is invalid")

// TokenAdrr is a simple struct to store a node token and its address.
type TokenAddr struct {
	Token crypto.Token
	Addr  string
}

// Message is used to channel listener messages to a general purpose channel.
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

// Dial tries to establish a signed connection to the given address. Hostname
// should be "localhost" or "" for intertnet connections. Should be anything
// else for local machine connections for testing. Address must have the form
// "address:port". Credentials is the private key of the party dialing. Token is
// the token of the party beeing dialed. It returns the signed connection or
// a nil and an errror.
func Dial(hostname, address string, credentials crypto.PrivateKey, token crypto.Token) (*SignedConnection, error) {
	var conn net.Conn
	var err error
	if hostname == "" || hostname == "localhost" {
		conn, err = net.Dial("tcp", address)
	} else {
		conn, err = TCPNetworkTest.Dial(hostname, address)
	}
	if err != nil {
		return nil, err
	}
	return performClientHandShake(conn, credentials, token)
}

func DialCtx(ctx context.Context, hostname, address string, credentials crypto.PrivateKey, token crypto.Token) (*SignedConnection, error) {
	var conn net.Conn
	var err error
	if hostname == "" || hostname == "localhost" {
		var d net.Dialer
		conn, err = d.DialContext(ctx, "tcp", address)
	} else {
		conn, err = TCPNetworkTest.Dial(hostname, address)
	}
	if err != nil {
		return nil, err
	}
	return performClientHandShake(conn, credentials, token)
}

// Listen returns a net.Listener. If addres is "localhost:port" or ":port" it
// returns the net.Listen("tcp", address). Otherwise it returns a TCPNetworkTest
// listener for testing. It returns nil and a error if it cannot bind on the
// on the port.
func Listen(address string) (net.Listener, error) {
	hostname, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if hostname == "" || hostname == "localhost" {
		return net.Listen("tcp", address)
	} else {
		return TCPNetworkTest.Listen(address)
	}
}

// SignedConnection is the key type of this package. It implements the
// Reader, Sender, Closer interface providing a simple interface to send and
// receive signed messages.
type SignedConnection struct {
	Token   crypto.Token
	Address string
	key     crypto.PrivateKey
	conn    net.Conn
	Live    bool
}

func (s *SignedConnection) Is(token crypto.Token) bool {
	return s.Token.Equal(token)
}

// Send up to 1<<32 - 1 bytes of data. It returns an error if the message is
// larger than 1<<32 - 1 bytes or if the underlying connection cannot send data.
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

// mustReadN reads exactly N bytes from the underlying connection. It returns
// an error if it cannot read N bytes or if the connection is closed before N
// bytes are read.
func (s *SignedConnection) mustReadN(N int) ([]byte, error) {
	buffer := make([]byte, 0, N)
	for {
		msg := make([]byte, N-len(buffer))
		nBytes, err := s.conn.Read(msg)
		if err != nil && err != io.EOF {
			return nil, err
		} else if err == io.EOF {
			buffer = append(buffer, msg[:nBytes]...)
			if len(buffer) == N {
				return buffer, nil
			} else {
				return nil, errors.New("signed connection read: EOF before full message received")
			}
		}
		if nBytes > 0 {
			buffer = append(buffer, msg[:nBytes]...)
			if len(buffer) == N {
				return buffer, nil
			}
		}
	}
}

// readWithoutCheck reads a message from the underlying connection without
// checking signature.
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

// Read reads a message from the underlying connection. It first reads the size
// of the message, than it reads the entire message and checks the signature.
// It returns an ErrInvalidSignature error if it could read but signature does
// not match.
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

// Helper function that Reads messages from the underlying connection and send
// them to the given channel identifying the token of the connection.
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

// Shutdown graciously closed the connection.
func (s *SignedConnection) Shutdown() {
	s.conn.Close()
	s.Live = false
}
