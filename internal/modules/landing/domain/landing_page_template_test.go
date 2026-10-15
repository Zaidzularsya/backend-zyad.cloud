package domain

import "testing"

func TestIsValidPageTemplateCategory(t *testing.T) {
	for _, c := range PageTemplateCategories {
		if !IsValidPageTemplateCategory(c) {
			t.Errorf("category %q should be valid", c)
		}
	}
	if len(PageTemplateCategories) != 12 {
		t.Fatalf("expected 12 categories, got %d", len(PageTemplateCategories))
	}
	for _, c := range []string{"", "Kesehatan", "lainnya"} {
		if IsValidPageTemplateCategory(c) {
			t.Errorf("category %q should be invalid", c)
		}
	}
}

func TestPageTemplateStatusIsValid(t *testing.T) {
	for _, s := range []PageTemplateStatus{PageTemplateStatusDraft, PageTemplateStatusPublished, PageTemplateStatusArchived} {
		if !s.IsValid() {
			t.Errorf("status %q should be valid", s)
		}
	}
	if PageTemplateStatus("deleted").IsValid() {
		t.Error("status deleted should be invalid")
	}
}
