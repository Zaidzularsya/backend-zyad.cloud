package model

import "time"

type Session struct {
	ID               string
	UserID           string
	RefreshTokenHash string
	DeviceName       string
	UserAgent        string
	IPAddress        string
	LastUsedAt       *time.Time
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	CreatedAt        time.Time
}

func (s Session) IsRevoked() bool {
	return s.RevokedAt != nil
}

func (s Session) IsExpired(now time.Time) bool {
	return !s.ExpiresAt.After(now)
}

func (s Session) IsActive(now time.Time) bool {
	return !s.IsRevoked() && !s.IsExpired(now)
}
