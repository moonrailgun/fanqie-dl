// SPDX-License-Identifier: GPL-3.0-only
// Adapted from ch6vip/fqapp rust/src/upstream/dh.rs.
package fanqie

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"math/big"
	"unicode/utf8"
)

var dhPrime, _ = new(big.Int).SetString("2410312426921032588552076022197566074856950548502459942654116941958108831682612228890093858261341614673227141477904012196503648957050582631942730706805009223062734745341073406696246014589361659774041027169249453200378729434170325843778659198143763193776859869524088940195577346119843545301547043747207749969763750084308926339295559968882457872412993810129130294592999947926365264059284647209730384947211681434464714438488520940127459844288859336526896320919633919", 10)
var requestKey, _ = base64.StdEncoding.DecodeString("rCXGfd2POMGzeiNIgo4iLg==")

type handshake struct{ private *big.Int }

func pad(data []byte) []byte {
	n := aes.BlockSize - len(data)%aes.BlockSize
	return append(append([]byte(nil), data...), bytes.Repeat([]byte{byte(n)}, n)...)
}

func newHandshake() (string, handshake, error) {
	return newDHHandshake(true)
}

func newDHHandshake(javaEncoding bool) (string, handshake, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", handshake{}, err
	}
	state := handshake{new(big.Int).SetBytes(secret)}
	pub := new(big.Int).Exp(big.NewInt(2), state.private, dhPrime).Bytes()
	// Java BigInteger encodes a leading zero for positive values with the high bit set.
	if javaEncoding && len(pub) > 0 && pub[0]&0x80 != 0 {
		pub = append([]byte{0}, pub...)
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", handshake{}, err
	}
	block, err := aes.NewCipher(requestKey)
	if err != nil {
		return "", handshake{}, err
	}
	plain := pad(pub)
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, plain)
	return base64.StdEncoding.EncodeToString(append(iv, ct...)), state, nil
}

func decryptCBC(data, key []byte) ([]byte, error) {
	if len(data) < 2*aes.BlockSize || len(data)%aes.BlockSize != 0 {
		return nil, errors.New("invalid encrypted content length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(data)-aes.BlockSize)
	cipher.NewCBCDecrypter(block, data[:aes.BlockSize]).CryptBlocks(plain, data[aes.BlockSize:])
	n := int(plain[len(plain)-1])
	if n < 1 || n > aes.BlockSize || !bytes.Equal(plain[len(plain)-n:], bytes.Repeat([]byte{byte(n)}, n)) {
		return nil, errors.New("invalid content padding")
	}
	return plain[:len(plain)-n], nil
}

func (s handshake) decrypt(peer, content string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(peer)
	if err != nil {
		return "", err
	}
	y := new(big.Int).SetBytes(raw)
	if y.Cmp(big.NewInt(1)) <= 0 || y.Cmp(new(big.Int).Sub(dhPrime, big.NewInt(1))) >= 0 {
		return "", errors.New("invalid server public key")
	}
	shared := new(big.Int).Exp(y, s.private, dhPrime).Bytes()
	if len(shared) < 32 {
		return "", errors.New("shared secret too short")
	}
	data, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return "", err
	}
	plain, err := decryptCBC(data, shared[:32])
	if err != nil {
		return "", err
	}
	if len(plain) == 0 || !utf8.Valid(plain) {
		return "", errors.New("decrypted content is empty or invalid UTF-8")
	}
	return string(plain), nil
}
