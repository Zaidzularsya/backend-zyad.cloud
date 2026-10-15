package publiclink

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database/testutil"
)

type fakeRow struct {
	link Link
	hash []byte
	enc  string
}

type fakeRepo struct{ rows []*fakeRow }

func newFakeRepo() *fakeRepo { return &fakeRepo{} }

func (f *fakeRepo) FindActive(_ context.Context, s coretenant.Scope, dt, did string) (Link, string, error) {
	for _, r := range f.rows {
		if r.link.OrganizationID == s.OrganizationID() && r.link.DocumentType == dt && r.link.DocumentID == did && r.link.RevokedAt == nil {
			return r.link, r.enc, nil
		}
	}
	return Link{}, "", pgx.ErrNoRows
}

func (f *fakeRepo) Insert(_ context.Context, s coretenant.Scope, dt, did string, hash []byte, enc string, exp time.Time, _ string) (Link, error) {
	l := Link{ID: "l" + string(rune('0'+len(f.rows))), OrganizationID: s.OrganizationID(), DocumentType: dt, DocumentID: did, ExpiresAt: exp}
	f.rows = append(f.rows, &fakeRow{link: l, hash: hash, enc: enc})
	return l, nil
}

func (f *fakeRepo) UpdateExpiry(_ context.Context, _ coretenant.Scope, id string, exp time.Time) error {
	for _, r := range f.rows {
		if r.link.ID == id {
			r.link.ExpiresAt = exp
		}
	}
	return nil
}

func (f *fakeRepo) Revoke(_ context.Context, s coretenant.Scope, dt, did string, at time.Time) error {
	for _, r := range f.rows {
		if r.link.OrganizationID == s.OrganizationID() && r.link.DocumentType == dt && r.link.DocumentID == did && r.link.RevokedAt == nil {
			r.link.RevokedAt = &at
		}
	}
	return nil
}

func (f *fakeRepo) FindByHash(_ context.Context, hash []byte) (Link, error) {
	for _, r := range f.rows {
		if bytes.Equal(r.hash, hash) {
			return r.link, nil
		}
	}
	return Link{}, pgx.ErrNoRows
}

func (f *fakeRepo) Touch(context.Context, string, time.Time) error { return nil }

func TestEnsureReusesActiveLinkAndKeepsToken(t *testing.T) {
	ctx := context.Background()
	scopeA := testutil.NewTenantPair(t).A.Scope
	svc := NewService(newFakeRepo(), "secret-key")
	exp := time.Date(2026, 11, 30, 16, 59, 59, 0, time.UTC)
	a, err := svc.Ensure(ctx, scopeA, DocumentQuotation, "q1", exp, "u1")
	if err != nil || a.Token == "" {
		t.Fatal(err)
	}
	b, _ := svc.Ensure(ctx, scopeA, DocumentQuotation, "q1", exp.Add(24*time.Hour), "u1")
	if b.ID != a.ID || b.Token != a.Token || !b.ExpiresAt.Equal(exp.Add(24*time.Hour)) {
		t.Fatalf("a=%+v b=%+v", a, b)
	}
	got, err := svc.Resolve(ctx, a.Token)
	if err != nil || got.DocumentID != "q1" || got.OrganizationID != scopeA.OrganizationID() || got.Token != "" {
		t.Fatalf("resolve=%+v err=%v", got, err)
	}
	_ = svc.Revoke(ctx, scopeA, DocumentQuotation, "q1")
	if r, err := svc.Resolve(ctx, a.Token); err != nil || r.RevokedAt == nil {
		t.Fatalf("revoked link must resolve with RevokedAt set: %+v err=%v", r, err)
	}
	c, _ := svc.Ensure(ctx, scopeA, DocumentQuotation, "q1", exp, "u1")
	if c.Token == a.Token {
		t.Fatal("after revoke a new token is issued")
	}
	if _, err := svc.Resolve(ctx, "nope"); !errors.Is(err, ErrLinkInvalid) {
		t.Fatal("unknown token")
	}
}
