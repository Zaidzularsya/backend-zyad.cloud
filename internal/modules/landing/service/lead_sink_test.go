package service

import (
	"errors"
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
)

func standardTestForm() domain.LandingForm {
	f := func(key, label string, order int) domain.LandingFormField {
		return domain.LandingFormField{Key: key, Label: label, SortOrder: order}
	}
	return domain.LandingForm{
		ID: "form-1",
		Fields: []domain.LandingFormField{
			f("name", "Nama lengkap", 0),
			f("email", "Email kerja", 1),
			f("phone", "No. WhatsApp", 2),
			f("company", "Perusahaan", 3),
			f("company_size", "Jumlah karyawan", 4),
			f("interest", "Minat", 5),
			f("message", "Pesan", 6),
			f("consent", "Saya setuju", 7),
		},
	}
}

func TestBuildLeadNotes_OrderAndLabels(t *testing.T) {
	sub := domain.LandingSubmission{
		SubmittedData: map[string]any{
			"name":         "Budi",
			"email":        "budi@x.id",
			"phone":        "0812",
			"company":      "PT X",
			"company_size": "11–50",
			"interest":     "Small Business · Tahunan",
			"message":      "Halo",
			"consent":      true,
		},
		SourceURL: "https://zyad.cloud/harga",
		UTMSource: "google",
	}

	got := buildLeadNotes(standardTestForm(), sub)

	want := "Jumlah karyawan: 11–50\nMinat: Small Business · Tahunan\nPesan: Halo\nHalaman: https://zyad.cloud/harga\nUTM source: google"
	if got != want {
		t.Fatalf("notes mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildLeadNotes_SkipsEmptyValuesAndFallsBackToPageURLField(t *testing.T) {
	sub := domain.LandingSubmission{
		SubmittedData: map[string]any{
			"interest": "  ",
			"message":  "Halo",
			"page_url": "https://zyad.cloud/x",
		},
	}

	got := buildLeadNotes(standardTestForm(), sub)

	want := "Pesan: Halo\nHalaman: https://zyad.cloud/x"
	if got != want {
		t.Fatalf("notes mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildLeadSinkInput_OwnerFallbackToPageCreator(t *testing.T) {
	form := standardTestForm()
	page := domain.LandingPage{ID: "p1", CreatedBy: "user-page"}
	sub := domain.LandingSubmission{ID: "s1", SubmittedData: map[string]any{
		"name": " Budi ", "email": "budi@x.id", "phone": "0812", "company": "PT X",
	}}

	in, err := buildLeadSinkInput(form, page, sub)
	if err != nil {
		t.Fatalf("buildLeadSinkInput() error = %v", err)
	}
	if in.OwnerUserID != "user-page" {
		t.Fatalf("OwnerUserID = %q, want user-page", in.OwnerUserID)
	}
	if in.SubmissionID != "s1" || in.Name != "Budi" || in.Email != "budi@x.id" || in.Phone != "0812" || in.Company != "PT X" {
		t.Fatalf("unexpected input: %+v", in)
	}

	form.LeadOwnerUserID = "user-form"
	in, err = buildLeadSinkInput(form, page, sub)
	if err != nil || in.OwnerUserID != "user-form" {
		t.Fatalf("form owner must win: owner=%q err=%v", in.OwnerUserID, err)
	}
}

func TestBuildLeadSinkInput_OwnerMissing(t *testing.T) {
	_, err := buildLeadSinkInput(standardTestForm(), domain.LandingPage{}, domain.LandingSubmission{})
	if !errors.Is(err, ErrLeadOwnerMissing) {
		t.Fatalf("error = %v, want ErrLeadOwnerMissing", err)
	}
}
