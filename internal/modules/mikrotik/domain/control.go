package domain

import "time"

// Isolation modes of a tenant (mikrotik_org_settings.isolation_mode).
const (
	ModeSimulation = "simulation"
	ModePilot      = "pilot"
	ModeActive     = "active"
	ModePaused     = "paused"
)

// MaxGraceDays is the upper bound of ControlSettings.GraceDays (DB check constraint).
const MaxGraceDays = 60

// ControlSettings is the tenant-level isolation configuration.
type ControlSettings struct {
	OrganizationID  string
	Mode            string
	GraceDays       int
	PilotVerifiedAt *time.Time
	UpdatedAt       time.Time
}

// PilotAccount is a customer account chosen for pilot-mode isolation.
type PilotAccount struct {
	AccountID     string
	AddedAt       time.Time
	TestStartedAt *time.Time
	TestUntil     *time.Time
}
