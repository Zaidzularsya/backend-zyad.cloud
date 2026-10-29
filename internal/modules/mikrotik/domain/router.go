package domain

import "time"

// APIType is the RouterOS API flavour used to talk to a router.
type APIType string

const (
	APITypeREST   APIType = "rest"
	APITypeBinary APIType = "binary"
)

// Router is a tenant-owned Mikrotik router. PasswordEncrypted is ciphertext and
// must never be returned to API clients.
type Router struct {
	ID                string
	OrganizationID    string
	Name              string
	Host              string
	Port              int
	APIType           APIType
	UseTLS            bool
	TLSFingerprint    string
	Username          string
	PasswordEncrypted string
	IsolirReady       bool
	IsolirReadyAt     *time.Time
	IsolirProfileName string
	IsolirRedirectURL string
	LastCheckAt       *time.Time
	LastCheckStatus   string
	RouterOSVersion   string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
