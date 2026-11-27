package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	socialdomain "zyad.cloud/internal/modules/social/domain"
	socialservice "zyad.cloud/internal/modules/social/service"
	userdto "zyad.cloud/internal/modules/user/dto"
)

// signupHookStore knows one referral code; RecordSignupReferral can be made to fail.
type signupHookStore struct {
	recorded []socialdomain.SignupReferral
	fail     error
}

func (s *signupHookStore) ReferrerByCode(_ context.Context, code string) (string, bool, error) {
	return "referrer", code == "ABCD2345", nil
}
func (s *signupHookStore) ReferralCode(context.Context, string) (string, error) { return "", nil }
func (s *signupHookStore) AssignReferralCode(_ context.Context, _, code string) (string, error) {
	return code, nil
}
func (s *signupHookStore) InsertVisit(context.Context, socialdomain.VisitEvent) error { return nil }
func (s *signupHookStore) RecordSignupReferral(_ context.Context, r socialdomain.SignupReferral) (bool, error) {
	if s.fail != nil {
		return false, s.fail
	}
	s.recorded = append(s.recorded, r)
	return true, nil
}
func (s *signupHookStore) CountEvents(context.Context, string) (socialdomain.EventCounts, error) {
	return socialdomain.EventCounts{}, nil
}

func TestSocialSignupHooksMapsAttributionAndSwallowsErrors(t *testing.T) {
	store := &signupHookStore{}
	var logs bytes.Buffer
	hooks := socialSignupHooks{
		attribution: socialservice.NewAttributionService(store, "https://zyad.test"),
		log:         slog.New(slog.NewTextHandler(&logs, nil)),
	}

	hooks.OnUserRegistered(context.Background(), "new-user", &userdto.AttributionInput{
		Code: "ABCD2345", Channel: "wa", FirstSeenAt: time.Now().Add(-time.Hour),
	})
	if len(store.recorded) != 1 || store.recorded[0].SubjectUserID != "new-user" || store.recorded[0].Channel != "wa" {
		t.Fatalf("recorded = %+v", store.recorded)
	}

	hooks.OnUserRegistered(context.Background(), "other-user", nil)
	if len(store.recorded) != 1 {
		t.Fatalf("nil attribution must be a no-op: %+v", store.recorded)
	}

	store.fail = errors.New("db down")
	hooks.OnUserRegistered(context.Background(), "third-user", &userdto.AttributionInput{Code: "ABCD2345"})
	if !strings.Contains(logs.String(), "record signup attribution failed") {
		t.Fatalf("failure must be logged, got %q", logs.String())
	}
}
