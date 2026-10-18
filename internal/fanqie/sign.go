// SPDX-License-Identifier: GPL-3.0-only
// Adapted from ch6vip/fqapp rust/src/sign/{argus,ladon,protobuf,simon,sm3}.rs.
package fanqie

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/bits"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func sm3(msg []byte) [32]byte {
	data := append(append([]byte(nil), msg...), 0x80)
	for len(data)%64 != 56 {
		data = append(data, 0)
	}
	data = binary.BigEndian.AppendUint64(data, uint64(len(msg))*8)
	v := [8]uint32{0x7380166f, 0x4914b2b9, 0x172442d7, 0xda8a0600, 0xa96f30bc, 0x163138aa, 0xe38dee4d, 0xb0fb0e4e}
	for off := 0; off < len(data); off += 64 {
		var w [68]uint32
		for i := 0; i < 16; i++ {
			w[i] = binary.BigEndian.Uint32(data[off+4*i:])
		}
		for i := 16; i < 68; i++ {
			x := w[i-16] ^ w[i-9] ^ bits.RotateLeft32(w[i-3], 15)
			w[i] = x ^ bits.RotateLeft32(x, 15) ^ bits.RotateLeft32(x, 23) ^ bits.RotateLeft32(w[i-13], 7) ^ w[i-6]
		}
		a, b, c, d, e, f, g, h := v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7]
		for j := 0; j < 64; j++ {
			tj, ff, gg := uint32(0x79cc4519), a^b^c, e^f^g
			if j >= 16 {
				tj = 0x7a879d8a
				ff = (a & b) | (a & c) | (b & c)
				gg = (e & f) | (^e & g)
			}
			ss1 := bits.RotateLeft32(bits.RotateLeft32(a, 12)+e+bits.RotateLeft32(tj, j), 7)
			ss2 := ss1 ^ bits.RotateLeft32(a, 12)
			t1 := ff + d + ss2 + (w[j] ^ w[j+4])
			t2 := gg + h + ss1 + w[j]
			d, c, b, a = c, bits.RotateLeft32(b, 9), a, t1
			h, g, f, e = g, bits.RotateLeft32(f, 19), e, t2^bits.RotateLeft32(t2, 9)^bits.RotateLeft32(t2, 17)
		}
		next := [8]uint32{a, b, c, d, e, f, g, h}
		for i := range v {
			v[i] ^= next[i]
		}
	}
	var out [32]byte
	for i, n := range v {
		binary.BigEndian.PutUint32(out[i*4:], n)
	}
	return out
}

func simon(pt [2]uint64, initial [4]uint64) [2]uint64 {
	var key [72]uint64
	copy(key[:], initial[:])
	for i := 4; i < 72; i++ {
		tmp := bits.RotateLeft64(key[i-1], -3) ^ key[i-3]
		tmp ^= bits.RotateLeft64(tmp, -1)
		key[i] = ^key[i-4] ^ tmp ^ ((uint64(0x3dc94c3a046d678b) >> ((i - 4) % 62)) & 1) ^ 3
	}
	x, y := pt[0], pt[1]
	for _, k := range key {
		x, y = y, x^(bits.RotateLeft64(y, 1)&bits.RotateLeft64(y, 8))^bits.RotateLeft64(y, 2)^k
	}
	return [2]uint64{x, y}
}

func ladon(ts, aid int64, random []byte) string {
	digest := md5.Sum(append(append([]byte(nil), random...), strconv.FormatInt(aid, 10)...))
	seed := []byte(hex.EncodeToString(digest[:]))
	var table [35]uint64
	for i := 0; i < 4; i++ {
		table[i] = binary.LittleEndian.Uint64(seed[i*8:])
	}
	a, b := table[0], table[1]
	queue := []uint64{table[2], table[3]}
	for i := 0; i < 34; i++ {
		x := (bits.RotateLeft64(b, -8) + a) ^ uint64(i)
		queue = append(queue, x)
		x ^= bits.RotateLeft64(a, 3)
		table[i+1] = x
		a, b = x, queue[0]
		queue = queue[1:]
	}
	data := pad([]byte(fmt.Sprintf("%d-1611921764-%d", ts, aid)))
	out := append([]byte(nil), random...)
	for i := 0; i < len(data); i += 16 {
		a, b := binary.LittleEndian.Uint64(data[i:]), binary.LittleEndian.Uint64(data[i+8:])
		for j := 0; j < 34; j++ {
			b = table[j] ^ (a + bits.RotateLeft64(b, -8))
			a = b ^ bits.RotateLeft64(a, 3)
		}
		out = binary.LittleEndian.AppendUint64(out, a)
		out = binary.LittleEndian.AppendUint64(out, b)
	}
	return base64.StdEncoding.EncodeToString(out)
}

type proto []byte

func (p *proto) number(field int, v uint64) {
	*p = binary.AppendUvarint(*p, uint64(field<<3))
	*p = binary.AppendUvarint(*p, uint64(uint32(v)))
}
func (p *proto) blob(field int, v []byte) {
	*p = binary.AppendUvarint(*p, uint64(field<<3|2))
	*p = binary.AppendUvarint(*p, uint64(len(v)))
	*p = append(*p, v...)
}
func (p *proto) text(field int, v string) { p.blob(field, []byte(v)) }

func argus(query string, body []byte, ts, aid int64, nonce uint32) string {
	q, _ := url.ParseQuery(query)
	var p proto
	p.number(1, 1077940818)
	p.number(2, 2)
	p.number(3, uint64(nonce))
	p.text(4, strconv.FormatInt(aid, 10))
	p.text(5, q.Get("device_id"))
	p.text(6, "1611921764")
	p.text(7, q.Get("version_name"))
	p.text(8, "v04.04.05-ov-android")
	p.number(9, 134744640)
	p.blob(10, make([]byte, 8))
	p.number(11, 0)
	p.number(12, uint64(ts<<1))
	bh, qh := sm3(make([]byte, 16)), sm3([]byte(query))
	if query == "" {
		qh = bh
	}
	if len(body) > 0 {
		digest := md5.Sum(body)
		bh = sm3(digest[:])
	}
	p.blob(13, bh[:6])
	p.blob(14, qh[:6])
	var sub proto
	sub.number(1, 1)
	sub.number(2, 1)
	sub.number(3, 1)
	sub.number(7, 0xc792eccc)
	p.blob(15, sub)
	p.text(16, "")
	p.text(20, "none")
	p.number(21, 738)
	sub = nil
	sub.text(1, "NX551J")
	sub.number(2, 8196)
	sub.number(4, 0x80e0d800)
	p.blob(23, sub)
	p.number(25, 2)
	key, _ := hex.DecodeString("fc78e0a9657a0c748ce51559903ccf03510e51d3cff232d71343e88a321c5304")
	var words [4]uint64
	for i := range words {
		words[i] = binary.LittleEndian.Uint64(key[i*8:])
	}
	padded := pad(p)
	encrypted := make([]byte, len(padded))
	for i := 0; i < len(padded); i += 16 {
		out := simon([2]uint64{binary.LittleEndian.Uint64(padded[i:]), binary.LittleEndian.Uint64(padded[i+8:])}, words)
		binary.LittleEndian.PutUint64(encrypted[i:], out[0])
		binary.LittleEndian.PutUint64(encrypted[i+8:], out[1])
	}
	magic := []byte{0xf2, 0xf7, 0xfc, 0xff, 0xf2, 0xf7, 0xfc, 0xff}
	framed := append(append(append([]byte(nil), magic...), encrypted...), 0xea, 0xfb, 0x2c, 0xfe, 0x85, 0x68, 0x51, 0x91, 0x54)
	for i := 8; i < len(framed); i++ {
		framed[i] ^= magic[i%8]
	}
	for i, j := 0, len(framed)-1; i < j; i, j = i+1, j-1 {
		framed[i], framed[j] = framed[j], framed[i]
	}
	framed = pad(append(framed, 'a', 'o'))
	aesKey, _ := hex.DecodeString("d31e3718288a1027baab59f146a09a9c")
	iv, _ := hex.DecodeString("ea180a0336ed352fcd24e4d50018ae54")
	block, _ := aes.NewCipher(aesKey)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(framed, framed)
	return base64.StdEncoding.EncodeToString(append([]byte{0xf2, 0x81}, framed...))
}

func sign(req *http.Request, body []byte) error {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	ts := time.Now().Unix()
	aid, _ := strconv.ParseInt(req.URL.Query().Get("aid"), 10, 64)
	if aid == 0 {
		aid = 1967
	}
	req.Header.Set("x-ss-req-ticket", strconv.FormatInt(ts*1000, 10))
	req.Header.Set("x-khronos", strconv.FormatInt(ts, 10))
	req.Header.Set("x-ladon", ladon(ts, aid, random[:4]))
	req.Header.Set("x-argus", argus(req.URL.RawQuery, body, ts, aid, binary.LittleEndian.Uint32(random[4:])))
	if len(body) > 0 {
		digest := md5.Sum(body)
		req.Header.Set("x-ss-stub", hex.EncodeToString(digest[:]))
	}
	return nil
}
