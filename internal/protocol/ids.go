package protocol

import (
	"crypto/rand"
	"encoding/hex"
)

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte("fallback-id-bytes!!"))
	}
	return hex.EncodeToString(b[:])
}

func NewToken(n int) string {
	if n <= 0 {
		n = 32
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return NewID()
	}
	return hex.EncodeToString(b)
}
