package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

var ErrInvalidTokenSize = errors.New("token size must be greater than zero")

func NewRandomToken(size int) (string, error) {
	if size <= 0 {
		return "", ErrInvalidTokenSize
	}

	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
