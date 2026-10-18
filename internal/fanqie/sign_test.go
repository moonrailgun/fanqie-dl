package fanqie

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"testing"
)

func TestSigningVectors(t *testing.T) {
	// Independent reference values preserved in fqapp rust/testdata/vectors.json.
	for input, want := range map[string]string{"": "1ab21d8355cfa17f8e61194831e81a8f22bec8c728fefb747ed035eb5082aa2b", "abc": "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"} {
		got := sm3([]byte(input))
		if hex.EncodeToString(got[:]) != want {
			t.Fatalf("SM3(%q)=%x", input, got)
		}
	}
	if got := simon([2]uint64{1234567890123456789, 9876543210987654321}, [4]uint64{1111111111111111111, 2222222222222222222, 3333333333333333333, 4444444444444444444}); got != [2]uint64{6071150433487837324, 2027321285990159935} {
		t.Fatal(got)
	}
	if got := ladon(1700000000, 1967, []byte{1, 2, 3, 4}); got != "AQIDBKM7Y7bWnOIUXmTybD6/MpGbuge4sBwUJsyHVivhtMsH" {
		t.Fatal(got)
	}
	// Nonce recovered from the upstream crypto_vectors.rs Argus551 golden output.
	want := "8oHyrVYEy1xDCDlx64ko40qC6JC9VYY/ZkjJzEl50iSFDw8Yfph1hupLxfKrA9GswSEz0Yi4goA7OQ4u+H8ECdOaQDL4GduQ3LULSq9Q4iSEGbS3L6G24GXChtnmPr6n46M2PLlKpXvF7uE0lhH4yD+X69FpdM41gQwx5Y5U3NdwXFaxlqYFszyp7dlUE6BEBWqwDsiAH8Jk9TGKr/jfHt4fVmMc9zRyNRdGeV0x6aP//x7aV+8Po8a/9KK3UNG5KEpmIgzRT6NUttkBP9gRNUh3"
	if got := argus("aid=1967&device_id=dev_aaaaaaaa&iid=111", nil, 1700000000, 1967, 659662259); got != want {
		t.Fatalf("Argus golden mismatch: %s", got)
	}
}

func TestSignPOSTBody(t *testing.T) {
	body := []byte(`{"book_id":"123","item_ids":["1","2"]}`)
	req, err := http.NewRequest(http.MethodPost, "https://example.test/?aid=3040", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := sign(req, body); err != nil {
		t.Fatal(err)
	}
	digest := md5.Sum(body)
	if req.Header.Get("x-ss-stub") != hex.EncodeToString(digest[:]) {
		t.Fatal("missing POST body digest")
	}
	if argus(req.URL.RawQuery, body, 1700000000, 3040, 1) == argus(req.URL.RawQuery, nil, 1700000000, 3040, 1) {
		t.Fatal("Argus ignored body")
	}
	got, _ := io.ReadAll(req.Body)
	if !bytes.Equal(got, body) {
		t.Fatal("signing consumed or changed request body")
	}
}
