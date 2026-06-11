package model

import "time"

type LoginEvent string

const (
	LoginEventLogin        LoginEvent = "login"
	LoginEventLogout       LoginEvent = "logout"
	LoginEventFailedLogin  LoginEvent = "failed_login"
	LoginEventTokenRefresh LoginEvent = "token_refresh"
	LoginEventTokenRevoked LoginEvent = "token_revoked"
)

type LoginHistory struct {
	ID         string
	UserID     string
	Identifier string
	Event      LoginEvent
	Success    bool
	IPAddress  string
	UserAgent  string
	DeviceName string
	Reason     string
	CreatedAt  time.Time
}
