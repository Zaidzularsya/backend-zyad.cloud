package model

import (
	"testing"
	"time"
)

func TestSessionIsActive(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	session := Session{ExpiresAt: now.Add(time.Hour)}

	if !session.IsActive(now) {
		t.Fatal("expected session to be active")
	}

	revokedAt := now.Add(time.Minute)
	session.RevokedAt = &revokedAt
	if session.IsActive(now) {
		t.Fatal("expected revoked session to be inactive")
	}
}

func TestSessionIsExpired(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	session := Session{ExpiresAt: now}

	if !session.IsExpired(now) {
		t.Fatal("expected session to be expired at expiry time")
	}
}
