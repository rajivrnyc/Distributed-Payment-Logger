package hash

import (
	"crypto/sha256"
	"encoding/hex"
)

func SHA256Hex(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
