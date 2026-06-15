package domain

import "testing"

func TestDomainValues(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{name: "page status", valid: PageStatusPublished.IsValid()},
		{name: "page visibility", valid: PageVisibilityPasswordProtected.IsValid()},
		{name: "page type", valid: PageTypePortfolioCaseStudy.IsValid()},
		{name: "section type", valid: SectionTypeHero.IsValid()},
		{name: "form field type", valid: FormFieldTypeFile.IsValid()},
		{name: "submission status", valid: SubmissionStatusQualified.IsValid()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.valid {
				t.Fatal("expected value to be valid")
			}
		})
	}
}

func TestInvalidDomainValues(t *testing.T) {
	if PageStatus("invalid").IsValid() {
		t.Fatal("invalid page status accepted")
	}
	if PageVisibility("invalid").IsValid() {
		t.Fatal("invalid page visibility accepted")
	}
	if PageType("invalid").IsValid() {
		t.Fatal("invalid page type accepted")
	}
	if SectionType("invalid").IsValid() {
		t.Fatal("invalid section type accepted")
	}
	if FormFieldType("invalid").IsValid() {
		t.Fatal("invalid form field type accepted")
	}
	if SubmissionStatus("invalid").IsValid() {
		t.Fatal("invalid submission status accepted")
	}
}
