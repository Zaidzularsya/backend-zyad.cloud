package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/social/domain"
)

type fakeAttributionStore struct {
	codes      map[string]string // userID -> referral code
	referredBy map[string]string // userID -> referrer userID
	taken      map[string]bool   // codes that collide on assignment
	visits     []domain.VisitEvent
	signups    []domain.SignupReferral
	counts     domain.EventCounts
	customers  []string // subjects credited with became_customer
}

func newFakeAttributionStore() *fakeAttributionStore {
	return &fakeAttributionStore{
		codes:      map[string]string{},
		referredBy: map[string]string{},
		taken:      map[string]bool{},
	}
}

func (f *fakeAttributionStore) ReferrerByCode(_ context.Context, code string) (string, bool, error) {
	for userID, c := range f.codes {
		if c == code {
			return userID, true, nil
		}
	}
	return "", false, nil
}

func (f *fakeAttributionStore) ReferralCode(_ context.Context, userID string) (string, error) {
	return f.codes[userID], nil
}

func (f *fakeAttributionStore) AssignReferralCode(_ context.Context, userID, code string) (string, error) {
	if existing := f.codes[userID]; existing != "" {
		return existing, nil
	}
	if f.taken[code] {
		return "", domain.ErrReferralCodeTaken
	}
	f.codes[userID] = code
	return code, nil
}

func (f *fakeAttributionStore) InsertVisit(_ context.Context, e domain.VisitEvent) error {
	f.visits = append(f.visits, e)
	return nil
}

func (f *fakeAttributionStore) RecordSignupReferral(_ context.Context, r domain.SignupReferral) (bool, error) {
	// Mirrors the repository: the referrer is written once and never replaced.
	if f.referredBy[r.SubjectUserID] != "" {
		return false, nil
	}
	f.referredBy[r.SubjectUserID] = r.ReferrerUserID
	f.signups = append(f.signups, r)
	return true, nil
}

func (f *fakeAttributionStore) CountEvents(_ context.Context, _ string) (domain.EventCounts, error) {
	return f.counts, nil
}

func (f *fakeAttributionStore) RecordBecameCustomer(_ context.Context, userID string) (bool, error) {
	f.customers = append(f.customers, userID)
	return true, nil
}

var attributionNow = time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)

func newTestAttributionService(store *fakeAttributionStore) *AttributionService {
	svc := NewAttributionService(store, "https://zyad.test/")
	svc.now = func() time.Time { return attributionNow }
	return svc
}

func TestRecordSignupFirstTouchWithin30Days(t *testing.T) {
	store := newFakeAttributionStore()
	store.codes["referrer"] = "ABCD2345"
	svc := newTestAttributionService(store)

	err := svc.RecordSignup(context.Background(), "new-user", &domain.AttributionInput{
		Code:        " abcd2345 ",
		Channel:     "whatsapp",
		PostID:      "0b6f3f53-7d5e-4a39-9f0e-3c1f9b7f3a10",
		FirstSeenAt: attributionNow.Add(-29 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("RecordSignup: %v", err)
	}
	if got := store.referredBy["new-user"]; got != "referrer" {
		t.Fatalf("referred_by = %q, want referrer", got)
	}
	if len(store.signups) != 1 {
		t.Fatalf("signups = %d, want 1", len(store.signups))
	}
	got := store.signups[0]
	if got.ReferralCode != "ABCD2345" || got.Channel != "whatsapp" || got.PostID != "0b6f3f53-7d5e-4a39-9f0e-3c1f9b7f3a10" {
		t.Fatalf("signup event = %+v", got)
	}
}

func TestRecordSignupIgnoresExpiredCode(t *testing.T) {
	store := newFakeAttributionStore()
	store.codes["referrer"] = "ABCD2345"
	svc := newTestAttributionService(store)

	err := svc.RecordSignup(context.Background(), "new-user", &domain.AttributionInput{
		Code:        "ABCD2345",
		FirstSeenAt: attributionNow.Add(-31 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("RecordSignup: %v", err)
	}
	if len(store.signups) != 0 || store.referredBy["new-user"] != "" {
		t.Fatalf("expired code must not attribute: %+v", store.signups)
	}
}

func TestRecordSignupIgnoresOwnCode(t *testing.T) {
	store := newFakeAttributionStore()
	store.codes["user-1"] = "ABCD2345"
	svc := newTestAttributionService(store)

	if err := svc.RecordSignup(context.Background(), "user-1", &domain.AttributionInput{
		Code: "ABCD2345", FirstSeenAt: attributionNow,
	}); err != nil {
		t.Fatalf("RecordSignup: %v", err)
	}
	if len(store.signups) != 0 {
		t.Fatalf("own code must be rejected: %+v", store.signups)
	}
}

func TestRecordSignupIgnoresUnknownCode(t *testing.T) {
	store := newFakeAttributionStore()
	svc := newTestAttributionService(store)

	for _, code := range []string{"ZZZZ9999", "not-a-code", ""} {
		if err := svc.RecordSignup(context.Background(), "new-user", &domain.AttributionInput{
			Code: code, FirstSeenAt: attributionNow,
		}); err != nil {
			t.Fatalf("RecordSignup(%q): %v", code, err)
		}
	}
	if err := svc.RecordSignup(context.Background(), "new-user", nil); err != nil {
		t.Fatalf("RecordSignup(nil): %v", err)
	}
	if len(store.signups) != 0 {
		t.Fatalf("unknown code must be ignored: %+v", store.signups)
	}
}

func TestRecordSignupNeverOverwritesReferrer(t *testing.T) {
	store := newFakeAttributionStore()
	store.codes["first"] = "ABCD2345"
	store.codes["second"] = "WXYZ6789"
	svc := newTestAttributionService(store)

	for _, code := range []string{"ABCD2345", "WXYZ6789"} {
		if err := svc.RecordSignup(context.Background(), "new-user", &domain.AttributionInput{
			Code: code, FirstSeenAt: attributionNow,
		}); err != nil {
			t.Fatalf("RecordSignup(%q): %v", code, err)
		}
	}
	if got := store.referredBy["new-user"]; got != "first" {
		t.Fatalf("referred_by = %q, want first", got)
	}
	if len(store.signups) != 1 {
		t.Fatalf("signups = %d, want 1", len(store.signups))
	}
}

func TestRecordSignupDropsInvalidPostIDAndLongChannel(t *testing.T) {
	store := newFakeAttributionStore()
	store.codes["referrer"] = "ABCD2345"
	svc := newTestAttributionService(store)

	if err := svc.RecordSignup(context.Background(), "new-user", &domain.AttributionInput{
		Code: "ABCD2345", Channel: strings.Repeat("x", 40), PostID: "nope",
	}); err != nil {
		t.Fatalf("RecordSignup: %v", err)
	}
	got := store.signups[0]
	if got.PostID != "" || len(got.Channel) != 20 {
		t.Fatalf("signup event = %+v", got)
	}
}

func TestEnsureCodeRetriesOnCollision(t *testing.T) {
	store := newFakeAttributionStore()
	store.taken["22222222"] = true
	svc := newTestAttributionService(store)
	// First draw collides (all zero bytes → "22222222"), second draw is free.
	svc.random = strings.NewReader(strings.Repeat("\x00", 8) + strings.Repeat("\x1f", 8))

	code, err := svc.EnsureCode(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("EnsureCode: %v", err)
	}
	if code != "ZZZZZZZZ" {
		t.Fatalf("code = %q, want ZZZZZZZZ", code)
	}
	again, err := svc.EnsureCode(context.Background(), "user-1")
	if err != nil || again != code {
		t.Fatalf("EnsureCode must be stable: %q, %v", again, err)
	}
}

func TestEnsureCodeGivesUpAfterFiveCollisions(t *testing.T) {
	store := newFakeAttributionStore()
	store.taken["22222222"] = true
	svc := newTestAttributionService(store)
	svc.random = strings.NewReader(strings.Repeat("\x00", 8*6))

	if _, err := svc.EnsureCode(context.Background(), "user-1"); !errors.Is(err, domain.ErrReferralCodeExhausted) {
		t.Fatalf("EnsureCode err = %v, want ErrReferralCodeExhausted", err)
	}
}

func TestRecordVisitHashesIPAndIgnoresUnknownCode(t *testing.T) {
	store := newFakeAttributionStore()
	store.codes["referrer"] = "ABCD2345"
	svc := newTestAttributionService(store)

	if err := svc.RecordVisit(context.Background(), VisitInput{Code: "QQQQ2222", IP: "203.0.113.9"}); err != nil {
		t.Fatalf("RecordVisit unknown: %v", err)
	}
	if len(store.visits) != 0 {
		t.Fatalf("unknown code must not insert: %+v", store.visits)
	}

	if err := svc.RecordVisit(context.Background(), VisitInput{
		Code: "abcd2345", Channel: "instagram", IP: "203.0.113.9", UserAgent: strings.Repeat("u", 300),
	}); err != nil {
		t.Fatalf("RecordVisit: %v", err)
	}
	if len(store.visits) != 1 {
		t.Fatalf("visits = %d, want 1", len(store.visits))
	}
	sum := sha256.Sum256([]byte("203.0.113.9"))
	got := store.visits[0]
	if got.IPHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("ip hash = %q", got.IPHash)
	}
	if got.ReferrerUserID != "referrer" || got.ReferralCode != "ABCD2345" || got.Channel != "instagram" {
		t.Fatalf("visit = %+v", got)
	}
	if len(got.UserAgent) != 200 {
		t.Fatalf("user agent length = %d, want 200", len(got.UserAgent))
	}
}

func TestSummaryBuildsShareURL(t *testing.T) {
	store := newFakeAttributionStore()
	store.codes["user-1"] = "ABCD2345"
	store.counts = domain.EventCounts{Visits: 7, Signups: 2}
	svc := newTestAttributionService(store)

	got, err := svc.Summary(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	want := Summary{Code: "ABCD2345", ShareURL: "https://zyad.test/r/ABCD2345", Visits: 7, Signups: 2}
	if got != want {
		t.Fatalf("Summary = %+v, want %+v", got, want)
	}
}

func TestRecordBecameCustomerDelegatesForUser(t *testing.T) {
	store := newFakeAttributionStore()
	svc := newTestAttributionService(store)
	if err := svc.RecordBecameCustomer(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordBecameCustomer(context.Background(), "user-1"); err != nil {
		t.Fatal(err)
	}
	if len(store.customers) != 1 || store.customers[0] != "user-1" {
		t.Fatalf("customers = %v", store.customers)
	}
}
