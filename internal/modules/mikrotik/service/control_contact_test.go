package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

func TestSetMode_PilotRequiresContact(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModeSimulation)
	e.prof.contact = TenantContact{Phone: "  ", Email: ""}
	_, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModePilot}, "u1")
	if !errors.Is(err, mikrotik.ErrTenantContactRequired) {
		t.Fatalf("err = %v, want ErrTenantContactRequired", err)
	}
	if e.repo.writes != 0 || e.repo.ctl.Mode != domain.ModeSimulation {
		t.Fatalf("nothing may be written: writes=%d mode=%s", e.repo.writes, e.repo.ctl.Mode)
	}
	for _, c := range []TenantContact{{Phone: "62812345"}, {Email: "a@b.c"}} {
		e := newCtlEnv(t, ModeOn, domain.ModeSimulation)
		e.prof.contact = c
		if _, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModePilot}, "u1"); err != nil {
			t.Fatalf("contact %+v: %v", c, err)
		}
	}
}

func TestSetMode_ActiveRequiresContact(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModePilot)
	v := ctlNow.Add(-time.Hour)
	e.repo.ctl.PilotVerifiedAt = &v
	e.prof.contact = TenantContact{}
	_, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModeActive}, "u1")
	if !errors.Is(err, mikrotik.ErrTenantContactRequired) {
		t.Fatalf("err = %v, want ErrTenantContactRequired", err)
	}
	if e.repo.ctl.Mode != domain.ModePilot {
		t.Fatalf("mode changed to %s", e.repo.ctl.Mode)
	}
}

func TestSetMode_SimulationAndPausedSkipProfile(t *testing.T) {
	for _, to := range []string{domain.ModeSimulation, domain.ModePaused} {
		e := newCtlEnv(t, ModeOn, domain.ModePilot)
		e.prof.err = errors.New("profile down")
		if _, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: to}, "u1"); err != nil {
			t.Fatalf("to %s: %v", to, err)
		}
	}
}

func TestSetMode_ProfileErrorFailsClosed(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModeSimulation)
	e.prof.err = errors.New("profile down")
	if _, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModePilot}, "u1"); err == nil {
		t.Fatal("want error when the profile cannot be read")
	}
	if e.repo.writes != 0 || e.repo.ctl.Mode != domain.ModeSimulation {
		t.Fatalf("mode must not change: writes=%d mode=%s", e.repo.writes, e.repo.ctl.Mode)
	}
}

func TestSetMode_NilProfileFailsClosed(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModeSimulation)
	e.svc.d.Profile = nil
	if _, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModePilot}, "u1"); !errors.Is(err, mikrotik.ErrTenantContactRequired) {
		t.Fatalf("err = %v", err)
	}
}
