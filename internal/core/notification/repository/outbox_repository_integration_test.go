//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestOutboxRepositoryLifecycleIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewOutboxRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventID := "00000000-0000-0000-0000-000000000501"
	event := domain.OutboxEvent{
		ID:        eventID,
		EventType: "auth.password_reset_requested",
		Recipient: domain.NotificationRecipient{
			Type:  "user",
			Email: "admin@example.test",
		},
		Payload: map[string]any{
			"app_name":   "Zyad Cloud",
			"user_name":  "Admin",
			"user_email": "admin@example.test",
			"reset_url":  "https://app.example.test/reset",
			"expired_at": "2026-06-12 10:00 WIB",
		},
		MaxAttempts: 3,
	}
	if err := repo.Enqueue(ctx, &event); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if event.ID != eventID {
		t.Fatalf("event.ID = %q, want %q", event.ID, eventID)
	}

	duplicate := event
	duplicate.Payload = map[string]any{"app_name": "Changed"}
	if err := repo.Enqueue(ctx, &duplicate); err != nil {
		t.Fatalf("Enqueue() duplicate error = %v", err)
	}

	claimed, err := repo.ClaimPending(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("len(claimed) = %d, want 1", len(claimed))
	}
	if claimed[0].Status != domain.OutboxStatusProcessing {
		t.Fatalf("claimed status = %q, want processing", claimed[0].Status)
	}
	if claimed[0].Payload["app_name"] != "Zyad Cloud" {
		t.Fatalf("claimed payload = %#v", claimed[0].Payload)
	}

	nextRetryAt := time.Now().UTC().Add(24 * time.Hour)
	if err := repo.MarkFailed(ctx, eventID, "send failed", &nextRetryAt); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	failed, err := repo.FindByID(ctx, eventID)
	if err != nil {
		t.Fatalf("FindByID() failed error = %v", err)
	}
	if failed.Status != domain.OutboxStatusFailed || failed.Attempts != 1 {
		t.Fatalf("failed event = %#v", failed)
	}

	claimed, err = repo.ClaimPending(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimPending() before retry time error = %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("len(claimed) before retry time = %d, want 0", len(claimed))
	}

	pastRetryAt := time.Now().UTC().Add(-time.Minute)
	if err := repo.MarkFailed(ctx, eventID, "retry now", &pastRetryAt); err == nil {
		t.Fatal("MarkFailed() from failed status error = nil, want error")
	}

	if err := repo.MarkDead(ctx, eventID, "dead"); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}
	dead, err := repo.FindByID(ctx, eventID)
	if err != nil {
		t.Fatalf("FindByID() dead error = %v", err)
	}
	if dead.Status != domain.OutboxStatusDead || dead.Attempts != dead.MaxAttempts {
		t.Fatalf("dead event = %#v", dead)
	}
}
