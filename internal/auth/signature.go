package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func HmacSHA256Hex(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func HmacEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
