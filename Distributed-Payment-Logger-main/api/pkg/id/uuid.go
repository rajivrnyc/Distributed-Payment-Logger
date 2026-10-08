package id

import (
	"crypto/rand"
	"encoding/hex"
)

func New() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexs := make([]byte, 36)
	hex.Encode(hexs[0:8], b[0:4])
	hexs[8] = '-'
	hex.Encode(hexs[9:13], b[4:6])
	hexs[13] = '-'
	hex.Encode(hexs[14:18], b[6:8])
	hexs[18] = '-'
	hex.Encode(hexs[19:23], b[8:10])
	hexs[23] = '-'
	hex.Encode(hexs[24:36], b[10:16])
	return string(hexs)
}
