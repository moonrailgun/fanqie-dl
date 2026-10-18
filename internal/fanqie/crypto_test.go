package fanqie

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"math/big"
	"testing"
)

func TestDHContent(t *testing.T) {
	y, state, err := newHandshake()
	if err != nil {
		t.Fatal(err)
	}
	want := "<p>第一章</p><p>这是完整正文。</p>"
	peer, encoded := serverContent(t, y, want)
	got, err := state.decrypt(peer, encoded)
	if err != nil || got != want {
		t.Fatalf("got %q, err %v", got, err)
	}
	if _, err := state.decrypt("AQ==", encoded); err == nil {
		t.Fatal("accepted degenerate peer")
	}
	if _, err := state.decrypt(peer, "AA=="); err == nil {
		t.Fatal("accepted truncated ciphertext")
	}
}

func serverContent(t *testing.T, y, body string) (string, string) {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(y)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := decryptCBC(raw, requestKey)
	if err != nil {
		t.Fatal(err)
	}
	serverSecret := big.NewInt(1234567)
	serverPub := new(big.Int).Exp(big.NewInt(2), serverSecret, dhPrime)
	shared := new(big.Int).Exp(new(big.Int).SetBytes(pub), serverSecret, dhPrime).Bytes()
	iv := make([]byte, aes.BlockSize)
	block, _ := aes.NewCipher(shared[:32])
	padded := pad([]byte(body))
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)
	encoded := base64.StdEncoding.EncodeToString(append(iv, ct...))
	return base64.StdEncoding.EncodeToString(serverPub.Bytes()), encoded
}

func TestCBCRejectsInvalidData(t *testing.T) {
	for _, data := range [][]byte{nil, make([]byte, 16), make([]byte, 17), make([]byte, 32)} {
		if _, err := decryptCBC(data, requestKey); err == nil {
			t.Fatalf("accepted %d bytes", len(data))
		}
	}
}
