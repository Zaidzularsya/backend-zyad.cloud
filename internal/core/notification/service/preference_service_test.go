package service

import (
	"context"
	"testing"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/dto"
)

func TestPreferenceServiceUpdateUserPreferences(t *testing.T) {
	repo := &fakePreferenceRepo{}
	service := NewPreferenceService(repo)

	preferences, err := service.UpdateUserPreferences(context.Background(), "user-1", "org-1", dto.BulkPreferenceRequest{
		Preferences: []dto.PreferenceRequest{
			{EventType: "payment.paid", Channel: "email", IsEnabled: true},
			{EventType: "payment.paid", Channel: "whatsapp", IsEnabled: false},
		},
	})
	if err != nil {
		t.Fatalf("UpdateUserPreferences() error = %v", err)
	}

	if len(preferences) != 2 {
		t.Fatalf("len(preferences) = %d, want 2", len(preferences))
	}
	if repo.saved[0].UserID != "user-1" || repo.saved[0].OrganizationID != "org-1" {
		t.Fatalf("saved preference = %#v", repo.saved[0])
	}
	if repo.saved[1].Channel != domain.ChannelWhatsApp || repo.saved[1].IsEnabled {
		t.Fatalf("saved preference = %#v", repo.saved[1])
	}
}

func TestPreferenceServiceUpdateRejectsDuplicatePreference(t *testing.T) {
	service := NewPreferenceService(&fakePreferenceRepo{})

	_, err := service.UpdateUserPreferences(context.Background(), "user-1", "", dto.BulkPreferenceRequest{
		Preferences: []dto.PreferenceRequest{
			{EventType: "payment.paid", Channel: "email", IsEnabled: true},
			{EventType: "payment.paid", Channel: "email", IsEnabled: false},
		},
	})
	if err == nil {
		t.Fatal("UpdateUserPreferences() error = nil, want error")
	}
}

func TestPreferenceServiceGetUserPreferences(t *testing.T) {
	repo := &fakePreferenceRepo{
		saved: []domain.NotificationPreference{
			{UserID: "user-1", EventType: "payment.paid", Channel: domain.ChannelEmail, IsEnabled: true},
		},
	}
	service := NewPreferenceService(repo)

	preferences, err := service.GetUserPreferences(context.Background(), "user-1", "")
	if err != nil {
		t.Fatalf("GetUserPreferences() error = %v", err)
	}
	if len(preferences) != 1 {
		t.Fatalf("len(preferences) = %d, want 1", len(preferences))
	}
}

type fakePreferenceRepo struct {
	saved []domain.NotificationPreference
}

func (r *fakePreferenceRepo) GetUserPreferences(context.Context, string, string) ([]domain.NotificationPreference, error) {
	result := make([]domain.NotificationPreference, len(r.saved))
	copy(result, r.saved)
	return result, nil
}

func (r *fakePreferenceRepo) BulkUpsertPreferences(_ context.Context, preferences []domain.NotificationPreference) error {
	r.saved = make([]domain.NotificationPreference, len(preferences))
	copy(r.saved, preferences)
	return nil
}
