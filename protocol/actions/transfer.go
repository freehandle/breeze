package actions

import (
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/util"
)

type Transfer struct {
	TimeStamp uint64
	From      crypto.Token
	To        []crypto.TokenValue
	Reason    string
	Fee       uint64
	Signature crypto.Signature
}

func (t *Transfer) Tokens() []crypto.Token {
	tokens := []crypto.Token{t.From}
	for _, to := range t.To {
		isNew := true
		for _, t := range tokens {
			if t.Equal(to.Token) {
				isNew = false
				break
			}
		}
		if isNew {
			tokens = append(tokens, to.Token)
		}
	}
	return tokens
}

func (t *Transfer) FeePaid() uint64 {
	return t.Fee
}

func (t *Transfer) serializeSign() []byte {
	bytes := []byte{0, ITransfer}
	util.PutUint64(t.TimeStamp, &bytes)
	util.PutToken(t.From, &bytes)
	util.PutUint16(uint16(len(t.To)), &bytes)
	count := len(t.To)
	if len(t.To) > 1<<16-1 {
		count = 1 << 16
	}
	for n := 0; n < count; n++ {
		util.PutToken(t.To[n].Token, &bytes)
		util.PutUint64(t.To[n].Value, &bytes)
	}
	util.PutString(t.Reason, &bytes)
	util.PutUint64(t.Fee, &bytes)
	return bytes
}

func (t *Transfer) Serialize() []byte {
	bytes := t.serializeSign()
	util.PutSignature(t.Signature, &bytes)
	return bytes
}

func (t *Transfer) Authority() crypto.Token {
	return crypto.ZeroToken
}

func (t *Transfer) Epoch() uint64 {
	return t.TimeStamp
}

func (t *Transfer) Kind() byte {
	return ITransfer
}

func (t *Transfer) Payments() *Payment {
	total := uint64(0)
	payment := &Payment{
		Credit: make([]Wallet, 0),
		Debit:  make([]Wallet, 0),
	}
	for _, credit := range t.To {
		payment.NewCredit(crypto.HashToken(credit.Token), credit.Value)
		total += credit.Value
	}
	payment.NewDebit(crypto.HashToken(t.From), total+t.Fee)
	return payment
}

func (t *Transfer) Sign(key crypto.PrivateKey) {
	bytes := t.serializeSign()
	t.Signature = key.Sign(bytes)
}

func (t *Transfer) JSON() string {
	bulk := &util.JSONBuilder{}
	bulk.PutString("kind", "transfer")
	bulk.PutUint64("version", 0)
	bulk.PutUint64("instructionType", uint64(ITransfer))
	bulk.PutUint64("epoch", t.TimeStamp)
	bulk.PutHex("from", t.From[:])
	bulk.PutTokenValueArray("to", t.To)
	bulk.PutString("reason", t.Reason)
	bulk.PutUint64("fee", t.Fee)
	bulk.PutBase64("signature", t.Signature[:])
	return bulk.ToString()
}

func ParseTransfer(data []byte) *Transfer {
	if len(data) < 2 || data[1] != ITransfer {
		return nil
	}
	p := Transfer{}
	position := 2
	p.TimeStamp, position = util.ParseUint64(data, position)
	p.From, position = util.ParseToken(data, position)
	var count uint16
	count, position = util.ParseUint16(data, position)
	p.To = make([]crypto.TokenValue, int(count))
	for i := 0; i < int(count); i++ {
		p.To[i].Token, position = util.ParseToken(data, position)
		p.To[i].Value, position = util.ParseUint64(data, position)
	}
	p.Reason, position = util.ParseString(data, position)
	p.Fee, position = util.ParseUint64(data, position)
	msg := data[0:position]
	p.Signature, _ = util.ParseSignature(data, position)
	if !p.From.Verify(msg, p.Signature) {
		return nil
	}
	return &p
}
