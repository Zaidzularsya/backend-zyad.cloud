package service

import (
	"testing"
	"time"

	"zyad.cloud/internal/core/businesstime"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

func TestIsolateAllowed_Table(t *testing.T) {
	modes := []string{domain.ModeSimulation, domain.ModePilot, domain.ModeActive, domain.ModePaused}
	type key struct {
		env, mode string
		pilot     bool
	}
	want := map[key]GateResult{}
	for _, m := range modes {
		for _, p := range []bool{false, true} {
			want[key{ModeOff, m, p}] = GateResult{false, ReasonEngineOff}
			want[key{"garbage", m, p}] = GateResult{false, ReasonEngineOff}
			want[key{ModeDryRun, m, p}] = GateResult{false, ReasonEngineDryRun}
			want[key{ModeOn, domain.ModeSimulation, p}] = GateResult{false, ReasonSimulation}
			want[key{ModeOn, domain.ModePaused, p}] = GateResult{false, ReasonPaused}
			want[key{ModeOn, domain.ModeActive, p}] = GateResult{true, ""}
		}
	}
	want[key{ModeOn, domain.ModePilot, false}] = GateResult{false, ReasonNotInPilot}
	want[key{ModeOn, domain.ModePilot, true}] = GateResult{true, ""}
	// An unknown tenant mode must never allow isolation.
	want[key{ModeOn, "bogus", true}] = GateResult{false, ReasonPaused}

	if ReasonEngineOff != "engine_off" || ReasonEngineDryRun != "engine_dry_run" || ReasonSimulation != "simulation" ||
		ReasonPaused != "paused" || ReasonNotInPilot != "not_in_pilot" {
		t.Fatal("reason constants changed")
	}
	for k, w := range want {
		got := IsolateAllowed(k.env, domain.ControlSettings{Mode: k.mode}, k.pilot)
		if got != w {
			t.Errorf("IsolateAllowed(%q, %q, pilot=%v) = %+v, want %+v", k.env, k.mode, k.pilot, got, w)
		}
	}
}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestEffectiveExempt_InclusiveUntil(t *testing.T) {
	today := date(2026, 11, 2)
	yesterday, tomorrow := date(2026, 11, 1), date(2026, 11, 3)
	cases := []struct {
		name string
		p    domain.CustomerPolicy
		want bool
	}{
		{"not exempt", domain.CustomerPolicy{}, false},
		{"not exempt with until", domain.CustomerPolicy{ExemptedUntil: &tomorrow}, false},
		{"nil until", domain.CustomerPolicy{Exempt: true}, true},
		{"until today", domain.CustomerPolicy{Exempt: true, ExemptedUntil: &today}, true},
		{"until tomorrow", domain.CustomerPolicy{Exempt: true, ExemptedUntil: &tomorrow}, true},
		{"until yesterday", domain.CustomerPolicy{Exempt: true, ExemptedUntil: &yesterday}, false},
	}
	for _, c := range cases {
		if got := EffectiveExempt(c.p, today); got != c.want {
			t.Errorf("%s: EffectiveExempt = %v, want %v", c.name, got, c.want)
		}
	}
	// The until day ends at WIB midnight: 23:30 WIB on the until day is still
	// exempt, 00:30 WIB the next day is not.
	until := date(2026, 11, 2)
	p := domain.CustomerPolicy{Exempt: true, ExemptedUntil: &until}
	if !EffectiveExempt(p, businesstime.DayOf(time.Date(2026, 11, 2, 16, 30, 0, 0, time.UTC))) {
		t.Error("23:30 WIB on the until day must be exempt")
	}
	if EffectiveExempt(p, businesstime.DayOf(time.Date(2026, 11, 2, 17, 30, 0, 0, time.UTC))) {
		t.Error("00:30 WIB after the until day must not be exempt")
	}
}

func TestCandidateCutoff_GraceBoundary(t *testing.T) {
	due := date(2026, 11, 10)
	// candidate mirrors the source rule: an invoice is a candidate when its
	// due date is before the cutoff passed as "today".
	candidate := func(now time.Time, grace int) bool {
		return due.Before(CandidateCutoff(businesstime.DayOf(now), grace))
	}
	// Last minute of T+3 in WIB (2026-11-13 23:59 WIB = 16:59Z).
	if candidate(time.Date(2026, 11, 13, 16, 59, 0, 0, time.UTC), 3) {
		t.Error("due T, grace 3: must not be a candidate on T+3")
	}
	// First minute of T+4 in WIB (2026-11-14 00:01 WIB = 2026-11-13 17:01Z).
	if !candidate(time.Date(2026, 11, 13, 17, 1, 0, 0, time.UTC), 3) {
		t.Error("due T, grace 3: must be a candidate on T+4 (WIB)")
	}
	// Grace 0 keeps the existing rule: not on the due day, yes the day after.
	if candidate(time.Date(2026, 11, 10, 5, 0, 0, 0, time.UTC), 0) || !candidate(time.Date(2026, 11, 11, 5, 0, 0, 0, time.UTC), 0) {
		t.Error("grace 0 boundary")
	}
	if got := CandidateCutoff(date(2026, 3, 2), 2); !got.Equal(date(2026, 2, 28)) {
		t.Errorf("cutoff across month = %v", got)
	}
	if got := CandidateCutoff(date(2026, 3, 2), -5); !got.Equal(date(2026, 3, 2)) {
		t.Errorf("negative grace must be treated as 0, got %v", got)
	}
}
