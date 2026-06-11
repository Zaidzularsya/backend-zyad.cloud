//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestTemplateRepositoryLifecycleIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewTemplateRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	code := testutil.UniqueCode("auth.password_reset")
	template := domain.NotificationTemplate{
		Code:            code,
		Name:            "Password Reset",
		Channel:         domain.ChannelEmail,
		Locale:          "id-ID",
		SubjectTemplate: "Reset password",
		BodyTemplate:    "Hello {{name}}, reset link: {{reset_link}}",
		AvailableVariables: []domain.NotificationVariable{
			{Key: "name", Description: "Recipient name", Required: true},
			{Key: "reset_link", Description: "Reset password link", Required: true},
		},
		SamplePayload: map[string]any{
			"name":       "Admin",
			"reset_link": "https://example.test/reset",
		},
		IsActive: true,
	}

	if err := repo.Create(ctx, &template); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if template.ID == "" {
		t.Fatal("Create() did not populate template ID")
	}
	if template.Status != domain.TemplateStatusDraft {
		t.Fatalf("Create() Status = %q, want %q", template.Status, domain.TemplateStatusDraft)
	}

	found, err := repo.FindByID(ctx, template.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if found.Code != code {
		t.Fatalf("FindByID() Code = %q, want %q", found.Code, code)
	}
	if len(found.AvailableVariables) != 2 {
		t.Fatalf("FindByID() AvailableVariables length = %d, want 2", len(found.AvailableVariables))
	}

	if err := repo.Activate(ctx, template.ID); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	active, err := repo.FindActiveByCodeChannelLocale(ctx, code, domain.ChannelEmail, "id-ID")
	if err != nil {
		t.Fatalf("FindActiveByCodeChannelLocale() error = %v", err)
	}
	if !active.CanBeUsed() {
		t.Fatal("FindActiveByCodeChannelLocale() returned template that cannot be used")
	}

	activeOnly := true
	list, err := repo.List(ctx, TemplateListFilter{
		Code:     code,
		Channel:  domain.ChannelEmail,
		IsActive: &activeOnly,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() length = %d, want 1", len(list))
	}

	if err := repo.SoftDelete(ctx, template.ID); err != nil {
		t.Fatalf("SoftDelete() error = %v", err)
	}

	if _, err := repo.FindActiveByCodeChannelLocale(ctx, code, domain.ChannelEmail, "id-ID"); err == nil {
		t.Fatal("FindActiveByCodeChannelLocale() expected error after soft delete")
	}
}
