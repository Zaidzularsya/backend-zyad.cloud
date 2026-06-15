package model

import "time"

type DomainType string

const (
	DomainTypePlatform  DomainType = "platform"
	DomainTypeSubdomain DomainType = "subdomain"
	DomainTypeCustom    DomainType = "custom"
)

func (t DomainType) IsValid() bool {
	return t == DomainTypePlatform || t == DomainTypeSubdomain || t == DomainTypeCustom
}

type DomainStatus string

const (
	DomainStatusPending  DomainStatus = "pending"
	DomainStatusVerified DomainStatus = "verified"
	DomainStatusActive   DomainStatus = "active"
	DomainStatusFailed   DomainStatus = "failed"
	DomainStatusDisabled DomainStatus = "disabled"
)

func (s DomainStatus) IsValid() bool {
	switch s {
	case DomainStatusPending,
		DomainStatusVerified,
		DomainStatusActive,
		DomainStatusFailed,
		DomainStatusDisabled:
		return true
	default:
		return false
	}
}

type DomainSSLStatus string

const (
	DomainSSLStatusPending      DomainSSLStatus = "pending"
	DomainSSLStatusProvisioning DomainSSLStatus = "provisioning"
	DomainSSLStatusActive       DomainSSLStatus = "active"
	DomainSSLStatusFailed       DomainSSLStatus = "failed"
	DomainSSLStatusNotRequired  DomainSSLStatus = "not_required"
)

func (s DomainSSLStatus) IsValid() bool {
	switch s {
	case DomainSSLStatusPending,
		DomainSSLStatusProvisioning,
		DomainSSLStatusActive,
		DomainSSLStatusFailed,
		DomainSSLStatusNotRequired:
		return true
	default:
		return false
	}
}

type OrganizationDomain struct {
	ID                        string
	OrganizationID            string
	Type                      DomainType
	CanonicalHost             string
	Status                    DomainStatus
	IsPrimary                 bool
	VerificationChallengeHash string
	VerificationAttempts      int
	LastVerificationAt        *time.Time
	VerifiedAt                *time.Time
	VerificationError         string
	SSLStatus                 DomainSSLStatus
	SSLError                  string
	SSLExpiresAt              *time.Time
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	DeletedAt                 *time.Time
}

type ReservedSubdomain struct {
	ID        string
	Label     string
	Reason    string
	IsSystem  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (d OrganizationDomain) CanResolvePublicly() bool {
	return d.DeletedAt == nil && d.Status == DomainStatusActive
}
