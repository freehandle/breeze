package actions

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

const voidTail = crypto.TokenSize + 8 + crypto.SignatureSize

type Void struct {
	TimeStamp uint64
	Protocol  uint32
	Data      []byte
	Wallet    crypto.Token
	Fee       uint64
	Signature crypto.Signature
}

func (v *Void) Tokens() []crypto.Token {
	return []crypto.Token{v.Wallet}
}

func (v *Void) FeePaid() uint64 {
	return v.Fee
}

func (t *Void) serializeSign() []byte {
	bytes := []byte{0, IVoid}
	util.PutUint64(t.TimeStamp, &bytes)
	util.PutUint32(t.Protocol, &bytes)
	bytes = append(bytes, t.Data...)
	//util.PutByteArray(t.Data, &bytes)
	util.PutToken(t.Wallet, &bytes)
	util.PutUint64(t.Fee, &bytes)
	return bytes
}

func (t *Void) JSON() string {
	bulk := &util.JSONBuilder{}
	bulk.PutString("kind", "void")
	bulk.PutUint64("version", 0)
	bulk.PutUint64("instructionType", uint64(IVoid))
	bulk.PutUint64("epoch", t.TimeStamp)
	bulk.PutUint64("protocol", uint64(t.Protocol))
	bulk.PutBase64("data", t.Data)
	bulk.PutUint64("fee", t.Fee)
	bulk.PutHex("wallet", t.Wallet[:])
	bulk.PutBase64("signature", t.Signature[:])
	return bulk.ToString()
}

func (t *Void) Serialize() []byte {
	bytes := t.serializeSign()
	util.PutSignature(t.Signature, &bytes)
	return bytes
}

func (t *Void) Epoch() uint64 {
	return t.TimeStamp
}

func (t *Void) Kind() byte {
	return IVoid
}

func (t *Void) Debit() Wallet {
	return Wallet{Account: crypto.HashToken(t.Wallet), FungibleTokens: t.Fee}
}

func (t *Void) Payments() *Payment {
	payment := &Payment{
		Credit: make([]Wallet, 0),
		Debit:  make([]Wallet, 0),
	}
	payment.NewDebit(crypto.HashToken(t.Wallet), t.Fee)
	return payment
}

func (t *Void) Sign(key crypto.PrivateKey) {
	bytes := t.serializeSign()
	t.Signature = key.Sign(bytes)
}

func ParseVoid(data []byte) *Void {
	if len(data) < 2 || data[1] != IVoid {
		return nil
	}
	p := Void{}
	position := 2
	p.TimeStamp, position = util.ParseUint64(data, position)
	p.Protocol, position = util.ParseUint32(data, position)
	if len(data)-voidTail < position {
		return nil
	}
	p.Data = data[position : len(data)-voidTail]
	//p.Data, position = util.ParseByteArray(data, position)
	position = len(data) - voidTail
	p.Wallet, position = util.ParseToken(data, position)
	p.Fee, position = util.ParseUint64(data, position)
	if position >= len(data) {
		return nil
	}
	msg := data[0:position]
	p.Signature, _ = util.ParseSignature(data, position)
	if !p.Wallet.Verify(msg, p.Signature) {
		return nil
	}
	return &p
}

func Dress(data []byte, wallet crypto.PrivateKey, fee uint64) []byte {
	output := make([]byte, len(data), len(data)+crypto.Size+8+crypto.SignatureSize)
	copy(output, data)
	util.PutToken(wallet.PublicKey(), &output)
	util.PutUint64(fee, &output)
	signature := wallet.Sign(output)
	util.PutSignature(signature, &output)
	return output
}
