package service

import (
	"context"
	"errors"
	"testing"

	"zyad.cloud/internal/modules/mikrotik"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

func TestFillDefaults_EmptyInput(t *testing.T) {
	got := SetupDefaults{PlatformHost: "api.zyad.cloud", PayHost: "zyad.cloud"}.Fill(SetupInput{})
	if got.PoolCIDR != "10.255.254.0/24" || got.PlatformHost != "api.zyad.cloud" {
		t.Fatalf("pool/platform = %q/%q", got.PoolCIDR, got.PlatformHost)
	}
	have := map[string]bool{}
	for _, h := range got.DokuHosts {
		have[h] = true
	}
	for _, want := range []string{"zyad.cloud", "staging.doku.com", "sandbox.doku.com", "api-sandbox.doku.com", "cdn-doku.oss-ap-southeast-5.aliyuncs.com"} {
		if !have[want] {
			t.Errorf("DokuHosts %v missing %s", got.DokuHosts, want)
		}
	}
}

func TestFillDefaults_NormalizesHosts(t *testing.T) {
	in := SetupDefaults{PlatformHost: "https://API.zyad.cloud:443/x", PayHost: "https://ZYAD.cloud:443/i/x"}.Fill(SetupInput{DokuHosts: []string{"zyad.cloud"}})
	n, err := normalizeSetup(in)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, h := range n.allowHosts {
		if h == "zyad.cloud" {
			count++
		}
	}
	if count != 1 || n.platformHost != "api.zyad.cloud" {
		t.Fatalf("allowHosts = %v platform=%s", n.allowHosts, n.platformHost)
	}
}

func TestFillDefaults_InputWins(t *testing.T) {
	in := SetupDefaults{PoolCIDR: "10.1.0.0/24", PlatformHost: "a.example.com", TenantIsolirHost: "t.example.com"}.
		Fill(SetupInput{PoolCIDR: "10.99.0.0/24", PlatformHost: "mine.example.com", TenantIsolirHost: "mine-t.example.com"})
	if in.PoolCIDR != "10.99.0.0/24" || in.PlatformHost != "mine.example.com" || in.TenantIsolirHost != "mine-t.example.com" {
		t.Fatalf("input overridden: %+v", in)
	}
}

func TestFillDefaults_ProductionHasNoSandboxHosts(t *testing.T) {
	in := SetupDefaults{PayHost: "zyad.cloud", DokuBaseURL: "https://api.doku.com"}.Fill(SetupInput{})
	for _, h := range in.DokuHosts {
		if h == "staging.doku.com" {
			t.Fatalf("sandbox host leaked into production: %v", in.DokuHosts)
		}
	}
}

func TestSetupPreview_OverlapReturns422(t *testing.T) {
	f := newSetupFixture(t)
	f.svc.opts.Defaults = SetupDefaults{PlatformHost: "api.zyad.cloud", PayHost: "zyad.cloud"}
	f.client.Seed(pathPool, pmk.Row{"name": "owner-pool", "ranges": "10.255.254.100-10.255.254.200"})
	_, err := f.svc.Preview(context.Background(), testScope(t), f.routerID, SetupInput{})
	if !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("err = %v, want invalid input (422), not an unclassified error", err)
	}
	if f.client.WriteCalls() != 0 {
		t.Fatal("wrote")
	}
}

func TestApply_SavesSetupSnapshot(t *testing.T) {
	f := newSetupFixture(t)
	in := validSetup()
	in.PoolCIDR = "10.99.0.0/24"
	in.RateLimit = "512k/512k"
	plan := f.preview(t, in)
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, in, plan.Hash, testActor); err != nil {
		t.Fatal(err)
	}
	got, err := f.repo.FindByID(context.Background(), testScope(t), f.routerID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Setup == nil || got.Setup.PoolCIDR != "10.99.0.0/24" || got.Setup.RateLimit != "512k/512k" || got.Setup.ProfileName != "zyad-isolir" {
		t.Fatalf("snapshot = %+v", got.Setup)
	}
}

func TestApply_FailureDoesNotSaveSnapshot(t *testing.T) {
	f := newSetupFixture(t)
	plan := f.preview(t, validSetup())
	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), "stale-"+plan.Hash, testActor); err == nil {
		t.Fatal("want ErrPlanChanged")
	}
	got, _ := f.repo.FindByID(context.Background(), testScope(t), f.routerID)
	if got.Setup != nil {
		t.Fatalf("snapshot saved on failure: %+v", got.Setup)
	}
}
