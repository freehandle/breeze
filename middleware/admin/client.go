package admin

import (
	"errors"
	"fmt"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/crypto/dh"
	"github.com/freehandle/breeze/socket"
)

type AdminClient struct {
	localEphemeral       crypto.PrivateKey
	remoteEphemeralToken crypto.Token
	conn                 *socket.SignedConnection
}

func DialAdmin(hostname string, node socket.TokenAddr, credential crypto.PrivateKey) (*AdminClient, error) {
	conn, err := socket.Dial(hostname, node.Addr, credential, node.Token)
	if err != nil {
		return nil, err
	}
	ephemeral, _ := dh.NewEphemeralKey()

	data, err := conn.Read()
	if err != nil {
		return nil, err
	}
	fmt.Println(data)
	if len(data) < crypto.TokenSize+1 || data[0] != EphemeralKey {
		return nil, errors.New("invalid ephemeral key")
	}

	admin := &AdminClient{
		localEphemeral: ephemeral,
		conn:           conn,
	}
	copy(admin.remoteEphemeralToken[:], data[1:])
	return admin, nil
}

func (a *AdminClient) Status() (string, error) {
	if err := a.conn.Send([]byte{MsgAdminReport}); err != nil {
		return "", err
	}
	data, err := a.conn.Read()
	if err != nil {
		return "", err
	}
	if len(data) < 1 || data[0] != StatusOk {
		return "", errors.New("invalid status")
	}
	return string(data[1:]), nil
}

func (a *AdminClient) SendSecret(key crypto.PrivateKey) error {
	cipher := dh.ConsensusCipher(a.localEphemeral, a.remoteEphemeralToken)
	data := cipher.Seal(key[:])
	localToken := a.localEphemeral.PublicKey()
	data = append(localToken[:], data...)
	if err := a.conn.Send(append([]byte{DiffieHellman}, data...)); err != nil {
		return err
	}
	data, err := a.conn.Read()
	if err != nil {
		return err
	}
	if len(data) < 1 || data[0] != StatusOk {
		return errors.New("could not exchange key")
	}
	return nil
}

func (a *AdminClient) FirewallAction(scope byte, token crypto.Token) error {
	msg := FirewallActionMessage(scope, token)
	if err := a.conn.Send(msg); err != nil {
		return err
	}
	data, err := a.conn.Read()
	if err != nil {
		return err
	}
	if len(data) < 1 || data[0] != StatusOk {
		return errors.New("firewall action rejected")
	}
	return nil
}

func (a *AdminClient) Activity(candidate bool) error {
	msg := []byte{Instruction, MsgActivation, 0}
	if candidate {
		msg[2] = 1
	}
	if err := a.conn.Send(msg); err != nil {
		return err
	}
	data, err := a.conn.Read()
	if err != nil {
		return err
	}
	if len(data) < 1 || data[0] != StatusOk {
		return errors.New("activity action rejected")
	}
	return nil
}
