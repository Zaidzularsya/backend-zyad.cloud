package service

import (
	"time"

	"zyad.cloud/internal/modules/mikrotik/domain"
)

// GateResult tells whether the engine may isolate an account right now.
// Reason is "" when allowed, otherwise one of the Reason* constants.
type GateResult struct {
	Allowed bool
	Reason  string
}

// Reasons an isolation is blocked. Restores are never gated.
const (
	ReasonEngineOff    = "engine_off"     // env kill switch: off
	ReasonEngineDryRun = "engine_dry_run" // env kill switch: dry_run
	ReasonSimulation   = "simulation"     // tenant mode simulation
	ReasonPaused       = "paused"         // tenant mode paused (or unknown)
	ReasonNotInPilot   = "not_in_pilot"   // tenant mode pilot, account not a pilot
	// ReasonControlUnreadable: the tenant control settings could not be read;
	// isolation fails closed, restores still run.
	ReasonControlUnreadable = "control_unreadable"
)

// modeControlUnreadable is the sentinel ControlSettings.Mode the evaluator
// uses when the settings read failed. It is not a stored tenant mode.
const modeControlUnreadable = "control_unreadable"

// IsolateAllowed combines the operator kill switch (env, the engine mode
// ModeOff/ModeDryRun/ModeOn; anything else counts as off) with the tenant
// rollout mode. The env always wins. An unknown tenant mode fails closed.
func IsolateAllowed(env string, ctl domain.ControlSettings, isPilot bool) GateResult {
	switch env {
	case ModeOn:
	case ModeDryRun:
		return GateResult{Reason: ReasonEngineDryRun}
	default:
		return GateResult{Reason: ReasonEngineOff}
	}
	switch ctl.Mode {
	case domain.ModeActive:
		return GateResult{Allowed: true}
	case domain.ModePilot:
		if isPilot {
			return GateResult{Allowed: true}
		}
		return GateResult{Reason: ReasonNotInPilot}
	case domain.ModeSimulation:
		return GateResult{Reason: ReasonSimulation}
	case modeControlUnreadable:
		return GateResult{Reason: ReasonControlUnreadable}
	default: // paused, or a value the engine does not know
		return GateResult{Reason: ReasonPaused}
	}
}

// EffectiveExempt reports whether an exemption protects the account on the
// business day today (a UTC-midnight date from businesstime.DayOf). The
// until date is inclusive: the exemption lasts through the end of that WIB day.
func EffectiveExempt(p domain.CustomerPolicy, today time.Time) bool {
	return p.Exempt && (p.ExemptedUntil == nil || !p.ExemptedUntil.Before(today))
}

// CandidateCutoff is the date passed as "today" to OverdueSource: an invoice
// is a candidate when due_date < today - graceDays, i.e.
// due_date + graceDays < today. A negative grace counts as 0.
func CandidateCutoff(today time.Time, graceDays int) time.Time {
	if graceDays < 0 {
		graceDays = 0
	}
	return today.AddDate(0, 0, -graceDays)
}
