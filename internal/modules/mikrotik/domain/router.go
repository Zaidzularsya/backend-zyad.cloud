package domain

import (
	"time"

	pmk "zyad.cloud/internal/platform/mikrotik"
)

// APIType is the RouterOS API flavour used to talk to a router.
type APIType string

const (
	APITypeREST   APIType = "rest"
	APITypeBinary APIType = "binary"
)

// MaxRouterPageSize is the largest router list page. The repository clamps to
// it and the handler reports the same effective size in pagination meta.
const MaxRouterPageSize = 200

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
	// Setup is the last successfully applied setup input, nil before the first apply.
	Setup           *SetupSnapshot
	LastCheckAt     *time.Time
	LastCheckStatus string
	RouterOSVersion string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SetupSnapshot is the operator-visible part of the last applied setup. Hosts
// are excluded on purpose: the allowlist is derived from platform config.
type SetupSnapshot struct {
	ProfileName string `json:"profile_name"`
	PoolCIDR    string `json:"pool_cidr"`
	RateLimit   string `json:"rate_limit"`
	RedirectURL string `json:"redirect_url"`
}

// Setup action kinds.
const (
	SetupCreate = "create"
	SetupUpdate = "update"
	SetupExists = "exists"
)

// Row is one RouterOS record (platform Row).
type Row = pmk.Row

// SetupAction is one planned change to a router object. Match holds the key
// attributes used to find the object; Attrs the desired attributes (for create
// it is exactly what is sent to the router, including place-before).
//
// Optional marks objects outside the default setup (HTTP redirect); Note is a
// human-readable warning for the UI. Both are part of the plan hash. ID is the
// router-side id of an existing object and is deliberately not hashed or
// serialized.
type SetupAction struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Match    Row    `json:"match"`
	Attrs    Row    `json:"attrs"`
	Optional bool   `json:"optional,omitempty"`
	Note     string `json:"note,omitempty"`
	ID       string `json:"-"`
}

// SetupPlan is the ordered list of actions plus the SHA-256 hex of their
// canonical JSON, used to make sure Apply executes exactly what was previewed.
type SetupPlan struct {
	Actions []SetupAction `json:"actions"`
	Hash    string        `json:"hash"`
}

// RouterStats is a live read of one router. Pointer fields are nil and string
// fields empty when the router did not answer that part.
type RouterStats struct {
	Identity, BoardName, Version, Architecture, Uptime string
	CPULoad                                            *int   // percent
	MemoryTotal, MemoryFree                            *int64 // bytes
	PPPActive                                          *int   // /ppp/active rows
	FetchedAt                                          time.Time
}

// RouterSummary is a database-only overview of one router: how many of its PPP
// secrets are in each state, and its isolation tasks by status. TaskCounts keys
// are task statuses; a status with no task is absent (read as 0).
type RouterSummary struct {
	SecretsTotal               int
	SecretsPaired              int
	SecretsUnpaired            int
	SecretsMissing             int
	SecretsNeedOriginalProfile int
	SecretsIsolated            int
	TaskCounts                 map[string]int
}
