package auth

import (
	"crypto/sha256"
	"encoding/hex"
)

func HashIP(ip, pepper string) string {
	sum := sha256.Sum256([]byte(pepper + "|" + ip))
	return hex.EncodeToString(sum[:])
}
