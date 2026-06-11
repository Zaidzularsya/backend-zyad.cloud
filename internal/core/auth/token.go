package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrMissingTokenSecret = errors.New("missing token secret")
	ErrInvalidToken       = errors.New("invalid token")
	ErrExpiredToken       = errors.New("expired token")
)

type TokenManager struct {
	secret []byte
	issuer string
	now    func() time.Time
}

func NewTokenManager(secret, issuer string) (*TokenManager, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrMissingTokenSecret
	}
	return &TokenManager{
		secret: []byte(secret),
		issuer: issuer,
		now:    time.Now,
	}, nil
}

func (m *TokenManager) Generate(claims Claims, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		return "", fmt.Errorf("token ttl must be greater than zero")
	}

	now := m.now().UTC()
	claims.Issuer = m.issuer
	claims.IssuedAt = now.Unix()
	claims.ExpiresAt = now.Add(ttl).Unix()
	if claims.Subject == "" {
		claims.Subject = claims.UserID
	}

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := encodedHeader + "." + encodedClaims
	signature := signHS256([]byte(signingInput), m.secret)

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (m *TokenManager) Parse(token string) (Claims, error) {
	var claims Claims

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, ErrInvalidToken
	}

	expectedSignature := signHS256([]byte(signingInput), m.secret)
	if !hmac.Equal(signature, expectedSignature) {
		return claims, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrInvalidToken
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, ErrInvalidToken
	}

	if claims.ExpiresAt <= m.now().UTC().Unix() {
		return claims, ErrExpiredToken
	}
	if m.issuer != "" && claims.Issuer != m.issuer {
		return claims, ErrInvalidToken
	}

	return claims, nil
}

func signHS256(input, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(input)
	return mac.Sum(nil)
}
