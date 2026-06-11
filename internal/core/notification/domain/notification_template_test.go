package domain

import (
	"testing"
	"time"
)

func TestTemplateStatusIsValid(t *testing.T) {
	valid := []TemplateStatus{
		TemplateStatusDraft,
		TemplateStatusActive,
		TemplateStatusInactive,
		TemplateStatusArchived,
	}

	for _, status := range valid {
		if !status.IsValid() {
			t.Fatalf("expected status %q to be valid", status)
		}
	}

	if TemplateStatus("unknown").IsValid() {
		t.Fatal("expected unknown template status to be invalid")
	}
}

func TestNotificationTemplateCanBeUsed(t *testing.T) {
	deletedAt := time.Now()

	tests := []struct {
		name     string
		template NotificationTemplate
		want     bool
	}{
		{
			name:     "active template can be used",
			template: NotificationTemplate{Status: TemplateStatusActive, IsActive: true},
			want:     true,
		},
		{
			name:     "draft template cannot be used",
			template: NotificationTemplate{Status: TemplateStatusDraft, IsActive: true},
			want:     false,
		},
		{
			name:     "inactive flag cannot be used",
			template: NotificationTemplate{Status: TemplateStatusActive, IsActive: false},
			want:     false,
		},
		{
			name:     "deleted template cannot be used",
			template: NotificationTemplate{Status: TemplateStatusActive, IsActive: true, DeletedAt: &deletedAt},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.template.CanBeUsed(); got != tt.want {
				t.Fatalf("CanBeUsed() = %v, want %v", got, tt.want)
			}
		})
	}
}
