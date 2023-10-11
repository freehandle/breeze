package util

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/freehandle/breeze/crypto"
)

type JSONBuilder struct {
	Encode strings.Builder
}

func (j *JSONBuilder) putGeneral(fieldName, value string) {
	if j.Encode.Len() > 0 {
		fmt.Fprintf(&j.Encode, `,"%v":%v`, fieldName, value)
	} else {
		fmt.Fprintf(&j.Encode, `"%v":%v`, fieldName, value)
	}
}

func (j *JSONBuilder) PutTime(fieldName string, t time.Time) {
	j.putGeneral(fieldName, t.Format(time.RFC3339))
}

func (j *JSONBuilder) PutUint64(fieldName string, value uint64) {
	j.putGeneral(fieldName, fmt.Sprintf("%v", value))
}

func (j *JSONBuilder) PutHex(fieldName string, value []byte) {
	if len(value) == 0 {
		return
	}
	j.putGeneral(fieldName, fmt.Sprintf(`"0x%v"`, hex.EncodeToString(value)))
}

func (j *JSONBuilder) PutBase64(fieldName string, value []byte) {
	if len(value) == 0 {
		return
	}
	j.putGeneral(fieldName, fmt.Sprintf(`"%v"`, base64.StdEncoding.EncodeToString(value)))
}

func (j *JSONBuilder) PutString(fieldName, value string) {
	j.putGeneral(fieldName, fmt.Sprintf(`"%v"`, value))
}

func (j *JSONBuilder) PutJSON(fieldName, value string) {
	j.putGeneral(fieldName, value)
}

func (j *JSONBuilder) ToString() string {
	return fmt.Sprintf(`{%v}`, j.Encode.String())
}

func (j *JSONBuilder) PutTokenValueArray(fieldName string, tokens []crypto.TokenValue) {
	if len(tokens) == 0 {
		return
	}
	array := &JSONBuilder{}
	array.Encode.WriteRune('[')
	first := true
	for _, r := range tokens {
		if !first {
			array.Encode.WriteRune(',')
		}
		fmt.Fprintf(&array.Encode, `{"token":"%v","value":%v`, base64.StdEncoding.EncodeToString(r.Token[:]), r.Value)
	}
	array.Encode.WriteRune(']')
	j.PutString(fieldName, array.Encode.String())
}

func (j *JSONBuilder) PutTokenCiphers(fieldName string, tc crypto.TokenCiphers) {
	if len(tc) == 0 {
		return
	}
	array := &JSONBuilder{}
	array.Encode.WriteRune('[')
	first := true
	for _, r := range tc {
		if !first {
			array.Encode.WriteRune(',')
		}
		fmt.Fprintf(&array.Encode, `{"token":"%v","cipher":%v`, base64.StdEncoding.EncodeToString(r.Token[:]), base64.StdEncoding.EncodeToString(r.Cipher))
	}
	array.Encode.WriteRune(']')
	j.PutString(fieldName, array.Encode.String())
}

func PrintJson(v any) {
	text, _ := json.Marshal(v)
	fmt.Println(string(text))
}
