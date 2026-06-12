//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	notificationtemplate "zyad.cloud/internal/core/notification/template"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestNotificationTemplateSeedIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewTemplateRepository(db)
	renderer := notificationtemplate.NewRenderer(nil)
	registry := notificationtemplate.NewVariableRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	expected := []struct {
		code    string
		channel domain.Channel
		locale  string
	}{
		{code: "auth.password_changed", channel: domain.ChannelEmail, locale: "id-ID"},
		{code: "auth.password_reset", channel: domain.ChannelEmail, locale: "id-ID"},
		{code: "user.invitation", channel: domain.ChannelEmail, locale: "id-ID"},
		{code: "lead.created", channel: domain.ChannelWhatsApp, locale: "id-ID"},
		{code: "payment.paid", channel: domain.ChannelEmail, locale: "id-ID"},
		{code: "security.new_login", channel: domain.ChannelEmail, locale: "id-ID"},
		{code: "permission.updated", channel: domain.ChannelEmail, locale: "id-ID"},
	}

	for _, item := range expected {
		template, err := repo.FindActiveByCodeChannelLocale(ctx, item.code, item.channel, item.locale)
		if err != nil {
			t.Fatalf("FindActiveByCodeChannelLocale(%s, %s) error = %v", item.code, item.channel, err)
		}
		if !template.IsSystem {
			t.Fatalf("%s IsSystem = false, want true", item.code)
		}
		if !template.CanBeUsed() {
			t.Fatalf("%s cannot be used", item.code)
		}

		assertSeedVariablesMatchRegistry(t, template, registry)
		rendered, err := renderer.Preview(template, nil)
		if err != nil {
			t.Fatalf("Preview(%s) error = %v", item.code, err)
		}
		if rendered.Body == "" {
			t.Fatalf("Preview(%s) rendered empty body", item.code)
		}
		if item.channel == domain.ChannelEmail && rendered.Subject == "" {
			t.Fatalf("Preview(%s) rendered empty subject", item.code)
		}
	}
}

func assertSeedVariablesMatchRegistry(t *testing.T, template domain.NotificationTemplate, registry *notificationtemplate.VariableRegistry) {
	t.Helper()

	expected := registry.Get(template.Code)
	if len(template.AvailableVariables) != len(expected) {
		t.Fatalf("%s AvailableVariables length = %d, want %d", template.Code, len(template.AvailableVariables), len(expected))
	}

	byKey := make(map[string]domain.NotificationVariable, len(template.AvailableVariables))
	for _, variable := range template.AvailableVariables {
		byKey[variable.Key] = variable
	}

	for _, variable := range expected {
		seeded, ok := byKey[variable.Key]
		if !ok {
			t.Fatalf("%s missing variable %s", template.Code, variable.Key)
		}
		if seeded.Required != variable.Required {
			t.Fatalf("%s variable %s Required = %v, want %v", template.Code, variable.Key, seeded.Required, variable.Required)
		}
	}
}
