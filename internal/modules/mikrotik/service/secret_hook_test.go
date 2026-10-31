package service

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

func TestSecretPair_TriggersEvaluation(t *testing.T) {
	e := newPairEnv(t)
	var got []string
	e.svc.SetOnChange(func(_ context.Context, _ coretenant.Scope, id string) { got = append(got, id) })
	id := e.importOne(t, "alice", "p")
	if _, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor); err != nil {
		t.Fatal(err)
	}
	id2 := e.importOne(t, "bob", "p")
	if _, err := e.svc.PairNewCustomer(context.Background(), e.scope, id2, testActor); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "acct-1" || got[1] != "acct-new" {
		t.Fatalf("onChange = %v", got)
	}
}

func TestSecretUnpair_TriggersEvaluation(t *testing.T) {
	e := newPairEnv(t)
	id := e.importOne(t, "alice", "p")
	if _, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor); err != nil {
		t.Fatal(err)
	}
	var got []string
	e.svc.SetOnChange(func(_ context.Context, _ coretenant.Scope, a string) { got = append(got, a) })
	if err := e.svc.Unpair(context.Background(), e.scope, id, testActor); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "acct-1" {
		t.Fatalf("onChange = %v", got)
	}
}

func TestSecretOnChange_PanicDoesNotFailPair(t *testing.T) {
	e := newPairEnv(t)
	e.svc.SetOnChange(func(context.Context, coretenant.Scope, string) { panic("boom") })
	id := e.importOne(t, "alice", "p")
	if _, err := e.svc.Pair(context.Background(), e.scope, id, "acct-1", testActor); err != nil {
		t.Fatalf("pair failed: %v", err)
	}
}
