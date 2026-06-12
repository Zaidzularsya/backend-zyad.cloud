package template

import (
	"errors"
	"testing"

	"zyad.cloud/internal/core/notification/domain"
)

func TestRendererRenderSubjectAndBody(t *testing.T) {
	renderer := NewRenderer(nil)

	rendered, err := renderer.Render(validPasswordResetTemplate(), map[string]any{
		"app_name":   "Zyad Cloud",
		"user_name":  "Admin",
		"reset_url":  "https://app.example.test/reset",
		"expired_at": "2026-06-12 10:00 WIB",
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if rendered.Subject != "Reset password for Zyad Cloud" {
		t.Fatalf("Render() Subject = %q", rendered.Subject)
	}
	if rendered.Body != "Hello Admin, open https://app.example.test/reset before 2026-06-12 10:00 WIB." {
		t.Fatalf("Render() Body = %q", rendered.Body)
	}
}

func TestRendererMissingRequiredVariableReturnsError(t *testing.T) {
	renderer := NewRenderer(nil)

	_, err := renderer.Render(validPasswordResetTemplate(), map[string]any{
		"app_name":  "Zyad Cloud",
		"user_name": "Admin",
		"reset_url": "https://app.example.test/reset",
	})
	if !errors.Is(err, ErrMissingRequiredVariable) {
		t.Fatalf("Render() error = %v, want ErrMissingRequiredVariable", err)
	}
}

func TestRendererOptionalVariableEmptyIsAllowed(t *testing.T) {
	renderer := NewRenderer(nil)
	template := domain.NotificationTemplate{
		Code:         "lead.created",
		Name:         "Lead Created",
		Channel:      domain.ChannelWhatsApp,
		Locale:       "id-ID",
		BodyTemplate: "Lead {{lead_name}} from {{lead_source}} phone {{lead_phone}} assigned to {{assigned_sales_name}}: {{crm_url}}",
		AvailableVariables: []domain.NotificationVariable{
			{Key: "app_name", Required: true},
			{Key: "lead_name", Required: true},
			{Key: "lead_source", Required: true},
			{Key: "lead_phone", Required: false},
			{Key: "assigned_sales_name", Required: false},
			{Key: "crm_url", Required: true},
		},
	}

	rendered, err := renderer.Render(template, map[string]any{
		"app_name":    "Zyad Cloud",
		"lead_name":   "Budi",
		"lead_source": "landing_page",
		"crm_url":     "https://app.example.test/admin/leads/123",
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if rendered.Body != "Lead Budi from landing_page phone  assigned to : https://app.example.test/admin/leads/123" {
		t.Fatalf("Render() Body = %q", rendered.Body)
	}
}

func TestRendererPreviewUsesSamplePayload(t *testing.T) {
	renderer := NewRenderer(nil)
	template := validPasswordResetTemplate()
	template.SamplePayload = map[string]any{
		"app_name":   "Zyad Cloud",
		"user_name":  "Admin",
		"reset_url":  "https://app.example.test/reset",
		"expired_at": "2026-06-12 10:00 WIB",
	}

	rendered, err := renderer.Preview(template, nil)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if rendered.Subject != "Reset password for Zyad Cloud" {
		t.Fatalf("Preview() Subject = %q", rendered.Subject)
	}
}
