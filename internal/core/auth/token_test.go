package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTokenManagerGenerateAndParse(t *testing.T) {
	manager := newTestTokenManager(t, time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC))

	token, err := manager.Generate(Claims{
		UserID:    "user-1",
		SessionID: "session-1",
		Email:     "admin@example.com",
		TokenType: TokenTypeAccess,
	}, time.Minute)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("expected user id user-1, got %s", claims.UserID)
	}
	if claims.SessionID != "session-1" {
		t.Fatalf("expected session id session-1, got %s", claims.SessionID)
	}
	if claims.Issuer != "zyad.cloud" {
		t.Fatalf("expected issuer zyad.cloud, got %s", claims.Issuer)
	}
}

func TestTokenManagerRejectsExpiredToken(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	manager := newTestTokenManager(t, now)

	token, err := manager.Generate(Claims{
		UserID:    "user-1",
		TokenType: TokenTypeAccess,
	}, time.Second)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	manager.now = func() time.Time { return now.Add(2 * time.Second) }
	if _, err := manager.Parse(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected expired token error, got %v", err)
	}
}

func TestTokenManagerRejectsInvalidSignature(t *testing.T) {
	manager := newTestTokenManager(t, time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC))

	token, err := manager.Generate(Claims{
		UserID:    "user-1",
		TokenType: TokenTypeAccess,
	}, time.Minute)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("expected compact jwt with three parts")
	}
	parts[2] = "invalid-signature"
	tamperedToken := strings.Join(parts, ".")

	if _, err := manager.Parse(tamperedToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token error, got %v", err)
	}
}

func TestNewTokenManagerRequiresSecret(t *testing.T) {
	if _, err := NewTokenManager("", "zyad.cloud"); !errors.Is(err, ErrMissingTokenSecret) {
		t.Fatalf("expected missing token secret error, got %v", err)
	}
}

func newTestTokenManager(t *testing.T, now time.Time) *TokenManager {
	t.Helper()

	manager, err := NewTokenManager("test-secret-with-enough-entropy", "zyad.cloud")
	if err != nil {
		t.Fatalf("new token manager: %v", err)
	}
	manager.now = func() time.Time { return now }
	return manager
}
