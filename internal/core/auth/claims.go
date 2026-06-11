package auth

type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

type Claims struct {
	UserID      string    `json:"uid"`
	SessionID   string    `json:"sid,omitempty"`
	Email       string    `json:"email,omitempty"`
	Username    string    `json:"username,omitempty"`
	Roles       []string  `json:"roles,omitempty"`
	Permissions []string  `json:"permissions,omitempty"`
	TokenType   TokenType `json:"typ"`
	Issuer      string    `json:"iss,omitempty"`
	Subject     string    `json:"sub,omitempty"`
	IssuedAt    int64     `json:"iat"`
	ExpiresAt   int64     `json:"exp"`
}
