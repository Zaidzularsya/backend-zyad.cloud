//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestPreferenceRepositoryLifecycleIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewPreferenceRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID := createNotificationPreferenceTestUser(t, ctx, db)
	organizationID := "00000000-0000-0000-0000-000000000101"

	enabled, err := repo.IsEnabled(ctx, userID, organizationID, "auth.password_reset", domain.ChannelEmail)
	if err != nil {
		t.Fatalf("IsEnabled() default error = %v", err)
	}
	if !enabled {
		t.Fatal("IsEnabled() default = false, want true")
	}

	globalPreference := domain.NotificationPreference{
		UserID:    userID,
		EventType: "auth.password_reset",
		Channel:   domain.ChannelEmail,
		IsEnabled: false,
	}
	if err := repo.UpsertPreference(ctx, &globalPreference); err != nil {
		t.Fatalf("UpsertPreference() global error = %v", err)
	}
	if globalPreference.ID == "" {
		t.Fatal("UpsertPreference() did not populate global preference ID")
	}

	enabled, err = repo.IsEnabled(ctx, userID, organizationID, "auth.password_reset", domain.ChannelEmail)
	if err != nil {
		t.Fatalf("IsEnabled() global fallback error = %v", err)
	}
	if enabled {
		t.Fatal("IsEnabled() global fallback = true, want false")
	}

	orgPreference := domain.NotificationPreference{
		UserID:         userID,
		OrganizationID: organizationID,
		EventType:      "auth.password_reset",
		Channel:        domain.ChannelEmail,
		IsEnabled:      true,
	}
	if err := repo.UpsertPreference(ctx, &orgPreference); err != nil {
		t.Fatalf("UpsertPreference() organization error = %v", err)
	}

	enabled, err = repo.IsEnabled(ctx, userID, organizationID, "auth.password_reset", domain.ChannelEmail)
	if err != nil {
		t.Fatalf("IsEnabled() organization override error = %v", err)
	}
	if !enabled {
		t.Fatal("IsEnabled() organization override = false, want true")
	}

	orgPreference.IsEnabled = false
	if err := repo.UpsertPreference(ctx, &orgPreference); err != nil {
		t.Fatalf("UpsertPreference() organization update error = %v", err)
	}

	preferences, err := repo.GetUserPreferences(ctx, userID, organizationID)
	if err != nil {
		t.Fatalf("GetUserPreferences() error = %v", err)
	}
	if len(preferences) != 1 {
		t.Fatalf("GetUserPreferences() length = %d, want 1", len(preferences))
	}
	if preferences[0].IsEnabled {
		t.Fatal("GetUserPreferences() IsEnabled = true, want false")
	}

	bulk := []domain.NotificationPreference{
		{
			UserID:         userID,
			OrganizationID: organizationID,
			EventType:      "auth.login",
			Channel:        domain.ChannelEmail,
			IsEnabled:      true,
		},
		{
			UserID:         userID,
			OrganizationID: organizationID,
			EventType:      "auth.login",
			Channel:        domain.ChannelWhatsApp,
			IsEnabled:      false,
		},
	}
	if err := repo.BulkUpsertPreferences(ctx, bulk); err != nil {
		t.Fatalf("BulkUpsertPreferences() error = %v", err)
	}

	preferences, err = repo.GetUserPreferences(ctx, userID, organizationID)
	if err != nil {
		t.Fatalf("GetUserPreferences() after bulk error = %v", err)
	}
	if len(preferences) != 3 {
		t.Fatalf("GetUserPreferences() after bulk length = %d, want 3", len(preferences))
	}
}

func createNotificationPreferenceTestUser(t *testing.T, ctx context.Context, db *database.Pool) string {
	t.Helper()

	email := fmt.Sprintf("notification-pref-%d@example.test", time.Now().UnixNano())
	var userID string
	err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status, created_at, updated_at)
		VALUES ('Notification Preference Test User', $1, 'active', now(), now())
		RETURNING id::text
	`, email).Scan(&userID)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	return userID
}
