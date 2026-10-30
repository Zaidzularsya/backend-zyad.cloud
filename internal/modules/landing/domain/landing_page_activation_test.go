package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"
)

func TestActivationStateTruthTable(t *testing.T) {
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	open := func(due time.Time) *LandingPageActivation {
		return &LandingPageActivation{Status: ActivationStatusOpen, InvoiceDueAt: due}
	}
	page := func(status PageStatus, activeUntil *time.Time) LandingPage {
		return LandingPage{Status: status, ActiveUntil: activeUntil}
	}

	cases := []struct {
		name string
		page LandingPage
		act  *LandingPageActivation
		want string
	}{
		{"draft, no activation", page(PageStatusDraft, nil), nil, ActivationStateNone},
		{"published regular page (active_until NULL)", page(PageStatusPublished, nil), nil, ActivationStateNone},
		{"published regular page with open activation", page(PageStatusPublished, nil), open(future), ActivationStatePreview},
		{"draft + open activation not yet due", page(PageStatusDraft, nil), open(future), ActivationStatePreview},
		{"draft + open activation past due", page(PageStatusDraft, nil), open(past), ActivationStateNone},
		{"draft + open activation due exactly now", page(PageStatusDraft, nil), open(now), ActivationStateNone},
		{"draft + non-open activation passed in", page(PageStatusDraft, nil), &LandingPageActivation{Status: ActivationStatusVoid, InvoiceDueAt: future}, ActivationStateNone},
		{"published + active_until future", page(PageStatusPublished, &future), nil, ActivationStatePublish},
		{"published + active_until future + open renewal", page(PageStatusPublished, &future), open(future), ActivationStatePublish},
		{"published + active_until past", page(PageStatusPublished, &past), nil, ActivationStateExpired},
		{"published + active_until exactly now", page(PageStatusPublished, &now), nil, ActivationStateExpired},
		{"expired + open renewal not yet due", page(PageStatusPublished, &past), open(future), ActivationStatePreview},
		{"expired + open renewal past due", page(PageStatusPublished, &past), open(past), ActivationStateExpired},
		{"unpublished + active_until past", page(PageStatusUnpublished, &past), nil, ActivationStateExpired},
		{"unpublished + active_until future", page(PageStatusUnpublished, &future), nil, ActivationStateNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ActivationState(tc.page, tc.act, now); got != tc.want {
				t.Fatalf("ActivationState = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPageActivationExpired(t *testing.T) {
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Second)

	if PageActivationExpired(LandingPage{}, now) {
		t.Fatal("active_until NULL must never be expired")
	}
	if !PageActivationExpired(LandingPage{ActiveUntil: &past}, now) {
		t.Fatal("active_until in the past must be expired")
	}
	if !PageActivationExpired(LandingPage{ActiveUntil: &now}, now) {
		t.Fatal("active_until == now must be expired")
	}
	if PageActivationExpired(LandingPage{ActiveUntil: &future}, now) {
		t.Fatal("active_until in the future must not be expired")
	}
}

func TestActivationPreviewable(t *testing.T) {
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		status ActivationStatus
		due    time.Time
		want   bool
	}{
		{ActivationStatusOpen, now.Add(time.Minute), true},
		{ActivationStatusOpen, now, false},
		{ActivationStatusOpen, now.Add(-time.Minute), false},
		{ActivationStatusPaid, now.Add(time.Minute), false},
		{ActivationStatusVoid, now.Add(time.Minute), false},
	}
	for _, tc := range cases {
		a := LandingPageActivation{Status: tc.status, InvoiceDueAt: tc.due}
		if got := a.PreviewableAt(now); got != tc.want {
			t.Fatalf("status=%s due=%s: PreviewableAt = %v, want %v", tc.status, tc.due, got, tc.want)
		}
	}
}

func TestNewActivationPreviewTokenIsRandomURLSafeAndHashed(t *testing.T) {
	raw, hash, err := NewActivationPreviewToken()
	if err != nil {
		t.Fatalf("NewActivationPreviewToken: %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("token is not url-safe base64: %v", err)
	}
	if len(decoded) < 32 {
		t.Fatalf("token entropy = %d bytes, want >= 32", len(decoded))
	}
	sum := sha256.Sum256([]byte(raw))
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash = %q, want sha256 hex of raw token", hash)
	}
	if hash == raw {
		t.Fatal("hash must differ from raw token")
	}
	if HashActivationPreviewToken(raw) != hash {
		t.Fatal("HashActivationPreviewToken must match the hash returned at creation")
	}

	raw2, _, err := NewActivationPreviewToken()
	if err != nil {
		t.Fatalf("NewActivationPreviewToken: %v", err)
	}
	if raw2 == raw {
		t.Fatal("two tokens must not collide")
	}
}

func TestActivationEnumsValid(t *testing.T) {
	for _, u := range []ActivationDurationUnit{ActivationDurationDay, ActivationDurationWeek, ActivationDurationMonth} {
		if !u.IsValid() {
			t.Fatalf("%q should be valid", u)
		}
	}
	if ActivationDurationUnit("year").IsValid() {
		t.Fatal("year must be invalid")
	}
	for _, s := range []ActivationStatus{ActivationStatusOpen, ActivationStatusPaid, ActivationStatusVoid} {
		if !s.IsValid() {
			t.Fatalf("%q should be valid", s)
		}
	}
	if ActivationStatus("expired").IsValid() {
		t.Fatal("expired is not a stored activation status")
	}
}

func TestSettleActivationPayment(t *testing.T) {
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	act := LandingPageActivation{DurationValue: 7, DurationUnit: ActivationDurationDay}
	later := now.Add(60 * 24 * time.Hour)
	del := now
	if s := SettleActivationPayment(LandingPage{Status: PageStatusArchived}, act, now); s.Publish {
		t.Fatal("archived page must not be published")
	}
	if s := SettleActivationPayment(LandingPage{Status: PageStatusDraft, DeletedAt: &del}, act, now); s.Publish {
		t.Fatal("deleted page must not be published")
	}
	if s := SettleActivationPayment(LandingPage{Status: PageStatusPublished, ActiveUntil: &later}, act, now); !s.ActiveUntil.Equal(later) {
		t.Fatalf("active_until = %v, want kept %v", s.ActiveUntil, later)
	}
}
