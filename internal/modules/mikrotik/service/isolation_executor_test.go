package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

const isolirProfile = "isolir"

type execEnv struct {
	ex     *Executor
	e      secretEnv
	secret domain.PPPSecret
}

func newExecEnv(t *testing.T, ready bool) execEnv {
	t.Helper()
	e := newSecretEnv(t)
	r := e.rrepo.routers[e.router.ID]
	r.IsolirReady, r.IsolirProfileName = ready, isolirProfile
	e.rrepo.routers[e.router.ID] = r
	e.client.Seed("/ppp/profile", pmk.Row{"name": "paket-10m"}, pmk.Row{"name": isolirProfile})
	e.seed(pmk.Row{"name": "alice", "profile": "paket-10m"})
	e.client.Seed("/ppp/active", pmk.Row{"name": "alice"}, pmk.Row{"name": "bob"})
	return execEnv{
		ex: NewExecutor(e.routers), e: e,
		secret: domain.PPPSecret{RouterID: e.router.ID, Name: "alice", OriginalProfile: "paket-10m"},
	}
}

func (x execEnv) profile(name string) string {
	for _, r := range x.e.client.Rows("/ppp/secret") {
		if r["name"] == name {
			return r["profile"]
		}
	}
	return ""
}

func (x execEnv) active(name string) int {
	n := 0
	for _, r := range x.e.client.Rows("/ppp/active") {
		if r["name"] == name {
			n++
		}
	}
	return n
}

func TestIsolate_SetsProfileRemovesActiveSessionsConfirms(t *testing.T) {
	x := newExecEnv(t, true)
	orig, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret)
	if err != nil || orig != "paket-10m" {
		t.Fatalf("orig=%q err=%v", orig, err)
	}
	if x.profile("alice") != isolirProfile || x.active("alice") != 0 || x.active("bob") != 1 {
		t.Fatalf("state: profile=%q alice=%d bob=%d", x.profile("alice"), x.active("alice"), x.active("bob"))
	}
}

func TestIsolate_AlreadyIsolated_NoSetKeepsOriginal(t *testing.T) {
	x := newExecEnv(t, true)
	if _, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret); err != nil {
		t.Fatal(err)
	}
	x.e.client.Seed("/ppp/active", pmk.Row{"name": "alice"})
	before := x.e.client.WriteCalls()
	orig, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret)
	if err != nil || orig != "paket-10m" {
		t.Fatalf("orig=%q err=%v", orig, err)
	}
	// Exactly one write: the session removal; no Set.
	if got := x.e.client.WriteCalls() - before; got != 1 {
		t.Fatalf("writes = %d, want 1", got)
	}
	if x.active("alice") != 0 {
		t.Fatal("sessions must still be cleared")
	}
}

func TestIsolate_RefreshesOriginalProfileWhenRouterProfileChanged(t *testing.T) {
	x := newExecEnv(t, true)
	x.secret.OriginalProfile = "old-profile"
	orig, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret)
	if err != nil || orig != "paket-10m" {
		t.Fatalf("orig=%q err=%v", orig, err)
	}
}

func TestIsolate_NoKnownOriginal_RestoreTargetInvalid(t *testing.T) {
	x := newExecEnv(t, true)
	x.secret.OriginalProfile = ""
	if _, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret); err != nil {
		t.Fatal(err) // router profile is the source
	}
	x.secret.Name = "alice"
	before := x.e.client.WriteCalls()
	_, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret) // now on isolir, nothing recorded
	if !errors.Is(err, mikrotik.ErrRestoreTargetInvalid) {
		t.Fatalf("err = %v", err)
	}
	if x.e.client.WriteCalls() != before {
		t.Fatal("must not write")
	}
}

func TestIsolate_SecretMissing_Permanent(t *testing.T) {
	x := newExecEnv(t, true)
	x.secret.Name = "ghost"
	_, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret)
	if !errors.Is(err, mikrotik.ErrSecretMissingOnRouter) || Classify(err).Kind != FailPermanent {
		t.Fatalf("err = %v", err)
	}
}

func TestIsolate_RouterNotReady_Permanent(t *testing.T) {
	x := newExecEnv(t, false)
	before := x.e.client.WriteCalls()
	_, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret)
	if !errors.Is(err, mikrotik.ErrRouterNotIsolirReady) || Classify(err).Kind != FailPermanent {
		t.Fatalf("err = %v", err)
	}
	if x.e.client.WriteCalls() != before {
		t.Fatal("must not write")
	}
}

// noopSetClient acknowledges Set without writing, modelling a router that
// accepts the command but does not apply the change.
type noopSetClient struct{ *pmk.FakeClient }

func (noopSetClient) Set(context.Context, string, string, pmk.Row) error { return nil }

type noopSetFactory struct {
	*fakeFactory
	c pmk.RouterClient
}

func (f noopSetFactory) New(string, pmk.Endpoint) (pmk.RouterClient, error) { return f.c, nil }

func TestIsolate_ConfirmMismatch_Transient(t *testing.T) {
	x := newExecEnv(t, true)
	// Rebuild the RouterService factory so the executor dials the wrapper.
	x.e.routers.factory = noopSetFactory{fakeFactory: &fakeFactory{client: x.e.client}, c: noopSetClient{x.e.client}}
	orig, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret)
	if !errors.Is(err, mikrotik.ErrUnreachable) || Classify(err).Kind != FailTransient {
		t.Fatalf("err = %v", err)
	}
	if orig != "" {
		t.Fatalf("original = %q, want empty on failure", orig)
	}
	if x.profile("alice") != "paket-10m" {
		t.Fatal("router profile should be untouched")
	}
}

func staleClient() pmk.RouterClient {
	c := pmk.NewFakeClient()
	c.Seed("/ppp/secret", pmk.Row{"name": "alice", "profile": "paket-10m"})
	return c
}

func TestRestore_ProfileMissingOnRouter_Permanent(t *testing.T) {
	x := newExecEnv(t, true)
	x.secret.OriginalProfile = "gone"
	before := x.e.client.WriteCalls()
	err := x.ex.Restore(context.Background(), x.e.scope, x.secret)
	if x.e.client.WriteCalls() != before || x.active("alice") != 1 {
		t.Fatal("must not set or drop sessions")
	}
	if !errors.Is(err, mikrotik.ErrRestoreProfileMissing) || Classify(err).Kind != FailPermanent {
		t.Fatalf("err = %v", err)
	}
}

func TestRestore_RefusesIsolirProfileAsTarget(t *testing.T) {
	x := newExecEnv(t, true)
	for _, p := range []string{isolirProfile, ""} {
		x.secret.OriginalProfile = p
		before := x.e.client.WriteCalls()
		if err := x.ex.Restore(context.Background(), x.e.scope, x.secret); !errors.Is(err, mikrotik.ErrRestoreTargetInvalid) {
			t.Fatalf("target %q: err = %v", p, err)
		}
		if x.e.client.WriteCalls() != before || x.active("alice") != 1 {
			t.Fatalf("target %q: must not write", p)
		}
	}
}

func TestRestore_WorksWhenRouterNotIsolirReady(t *testing.T) {
	x := newExecEnv(t, false)
	if _, err := x.e.client.Print(context.Background(), "/ppp/secret", nil); err != nil {
		t.Fatal(err)
	}
	// put alice on the isolir profile, with a live session
	for _, r := range x.e.client.Rows("/ppp/secret") {
		if r["name"] == "alice" {
			_ = x.e.client.Set(context.Background(), "/ppp/secret", r[".id"], pmk.Row{"profile": isolirProfile})
		}
	}
	if err := x.ex.Restore(context.Background(), x.e.scope, x.secret); err != nil {
		t.Fatal(err)
	}
	if x.profile("alice") != "paket-10m" || x.active("alice") != 0 {
		t.Fatalf("profile=%q active=%d", x.profile("alice"), x.active("alice"))
	}
}

func TestIsolate_UnreachableTransient_NoCredentialInError(t *testing.T) {
	x := newExecEnv(t, true)
	x.e.client.Err = errors.New("dial tcp: login with " + testPassword + " failed")
	_, err := x.ex.Isolate(context.Background(), x.e.scope, x.secret)
	if err == nil || strings.Contains(err.Error(), testPassword) {
		t.Fatalf("err = %v", err)
	}
	if f := Classify(err); f.Kind != FailTransient || strings.Contains(f.Message, testPassword) {
		t.Fatalf("failure = %+v", f)
	}
}
