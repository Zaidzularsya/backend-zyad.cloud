package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

var ErrInvalidCiphertext = errors.New("invalid ciphertext")

// deriveKey turns an arbitrary-length application secret into a 32-byte
// AES-256 key. It is not a substitute for a proper KMS/key-rotation setup —
// see EncryptSecret's doc comment.
func deriveKey(secret string) [32]byte {
	return sha256.Sum256([]byte(secret))
}

// EncryptSecret encrypts plaintext with AES-256-GCM, keyed from secret (the
// caller passes the application's config secret — e.g. cfg.App.Secret,
// already reused elsewhere in this codebase for signing, see
// landingservice.NewPublishService), and returns a base64-encoded string
// safe to store in a text column. Used by the CRM integration module to
// store connection secrets (API keys, webhook signing secrets) at rest —
// see internal/modules/crm/service/integration_service.go.
//
// This is a single-key, no-rotation scheme: if the app secret ever changes,
// every previously encrypted value becomes undecryptable. Acceptable for
// Fase 5's scope, but revisit (versioned keys, a real KMS) before this
// protects anything beyond low-sensitivity integration config.
func EncryptSecret(secret string, plaintext string) (string, error) {
	key := deriveKey(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptSecret reverses EncryptSecret.
func DecryptSecret(secret string, encoded string) (string, error) {
	key := deriveKey(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrInvalidCiphertext
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	return string(plaintext), nil
}
