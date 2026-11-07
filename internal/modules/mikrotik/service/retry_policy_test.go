package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"zyad.cloud/internal/modules/mikrotik"
)

func TestNextAttempt_BackoffTables(t *testing.T) {
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		action   string
		attempts int
		delay    time.Duration
		done     bool
	}{
		{ActionRestore, 0, 30 * time.Second, false}, {ActionIsolate, -1, time.Minute, false},
		{ActionRestore, 1, 30 * time.Second, false}, {ActionRestore, 2, time.Minute, false},
		{ActionRestore, 3, 2 * time.Minute, false}, {ActionRestore, 4, 5 * time.Minute, false},
		{ActionRestore, 5, 15 * time.Minute, false}, {ActionRestore, 6, 15 * time.Minute, false},
		{ActionRestore, 11, 15 * time.Minute, false}, {ActionRestore, 12, 0, true},
		{ActionIsolate, 1, time.Minute, false}, {ActionIsolate, 2, 5 * time.Minute, false},
		{ActionIsolate, 3, 15 * time.Minute, false}, {ActionIsolate, 4, time.Hour, false},
		{ActionIsolate, 5, time.Hour, false}, {ActionIsolate, 7, time.Hour, false},
		{ActionIsolate, 8, 0, true}, {ActionIsolate, 9, 0, true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s_%d", c.action, c.attempts), func(t *testing.T) {
			next, done := NextAttempt(c.action, c.attempts, now)
			if done != c.done {
				t.Fatalf("exhausted = %v, want %v", done, c.done)
			}
			if !done && !next.Equal(now.Add(c.delay)) {
				t.Fatalf("next = %v, want +%v", next, c.delay)
			}
		})
	}
}

func TestShouldNotifyTenant_ThresholdsDifferByAction(t *testing.T) {
	if ShouldNotifyTenant(ActionIsolate, 4) || !ShouldNotifyTenant(ActionIsolate, 5) {
		t.Fatal("isolate threshold must be 5")
	}
	if ShouldNotifyTenant(ActionRestore, 2) || !ShouldNotifyTenant(ActionRestore, 3) {
		t.Fatal("restore threshold must be 3")
	}
	if !ShouldNotifyTenant(ActionIsolate, 8) || !ShouldNotifyTenant(ActionRestore, 12) {
		t.Fatal("exhausted must notify")
	}
}

func TestClassify_RouterNotFound_Permanent(t *testing.T) {
	for _, err := range []error{mikrotik.ErrRouterNotFound, mikrotik.ErrRouterNotIsolirReady} {
		if f := Classify(fmt.Errorf("wrap: %w", err)); f.Kind != FailPermanent || f.Code == "" || f.Message == "" {
			t.Fatalf("%v -> %+v", err, f)
		}
	}
}

func TestClassify_KindsAndCodes(t *testing.T) {
	perm := map[error]string{
		mikrotik.ErrAuthFailed: "auth_failed", mikrotik.ErrTLSUntrusted: "tls_untrusted",
		&mikrotik.TLSUntrustedError{}: "tls_untrusted", mikrotik.ErrHostNotAllowed: "host_not_allowed",
		mikrotik.ErrSecretMissingOnRouter: "secret_missing", mikrotik.ErrRestoreProfileMissing: "profile_missing",
		mikrotik.ErrRestoreTargetInvalid: "restore_target_invalid",
	}
	for err, code := range perm {
		if f := Classify(err); f.Kind != FailPermanent || f.Code != code {
			t.Fatalf("%v -> %+v", err, f)
		}
	}
	if f := Classify(mikrotik.ErrUnreachable); f.Kind != FailTransient || f.Code != "unreachable" {
		t.Fatalf("unreachable -> %+v", f)
	}
	if f := Classify(errors.New("boom pw=secret")); f.Kind != FailTransient || f.Code != "unknown" || f.Message == "" {
		t.Fatalf("unknown -> %+v", f)
	}
}
