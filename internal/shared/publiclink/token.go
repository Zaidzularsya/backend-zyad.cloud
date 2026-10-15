// Package publiclink mengelola link publik bertoken untuk dokumen tenant
// (quotation, invoice) yang dibuka customer tanpa login.
package publiclink

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// NewToken membuat token 32 byte acak (base64url tanpa padding, 43 karakter)
// beserta sha256-nya, yang menjadi kunci pencarian di database.
func NewToken() (token string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
