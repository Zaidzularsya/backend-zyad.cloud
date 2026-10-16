package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

func rulesField(key string, typ domain.FormFieldType, required bool, order int, def string) domain.LandingFormField {
	return domain.LandingFormField{Key: key, Type: typ, IsRequired: required, SortOrder: order, DefaultValue: def, Label: key}
}

// legacyStandardFields = preset 8 field yang dibuat frontend sebelum rilis ini.
func legacyStandardFields() []domain.LandingFormField {
	return []domain.LandingFormField{
		rulesField("name", domain.FormFieldTypeText, true, 0, ""),
		rulesField("email", domain.FormFieldTypeEmail, true, 1, ""),
		rulesField("phone", domain.FormFieldTypePhone, true, 2, ""),
		rulesField("company", domain.FormFieldTypeText, false, 3, ""),
		rulesField("company_size", domain.FormFieldTypeSelect, false, 4, ""),
		rulesField("interest", domain.FormFieldTypeText, false, 5, ""),
		rulesField("message", domain.FormFieldTypeTextarea, false, 6, ""),
		rulesField("consent", domain.FormFieldTypeCheckbox, true, 7, ""),
	}
}

func TestPrepareSubmittedData_DropsUnknownKeys(t *testing.T) {
	out, err := prepareSubmittedData(legacyStandardFields(), map[string]any{
		"name": "Budi", "email": "b@x.id", "phone": "0812345678", "consent": true,
		"hack": "x", "page_url": "https://x.id/p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["hack"]; ok {
		t.Error("unknown key must be dropped")
	}
	if out["page_url"] != "https://x.id/p" || out["consent"] != true {
		t.Errorf("page_url/consent must be kept: %+v", out)
	}
}

func TestPrepareSubmittedData_HiddenDefaultApplied(t *testing.T) {
	fields := legacyStandardFields()
	fields[5] = rulesField("interest", domain.FormFieldTypeHidden, false, 5, "Paket 20 Mbps")
	out, err := prepareSubmittedData(fields, map[string]any{"name": "B", "email": "b@x.id", "phone": "0812345678", "consent": true})
	if err != nil {
		t.Fatal(err)
	}
	if out["interest"] != "Paket 20 Mbps" {
		t.Errorf("interest = %v", out["interest"])
	}
}

func TestPrepareSubmittedData_HiddenVisitorValueWins(t *testing.T) {
	fields := legacyStandardFields()
	fields[5] = rulesField("interest", domain.FormFieldTypeHidden, false, 5, "Paket 20 Mbps")
	out, err := prepareSubmittedData(fields, map[string]any{
		"name": "B", "email": "b@x.id", "phone": "0812345678", "consent": true, "interest": "Small Business · Tahunan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["interest"] != "Small Business · Tahunan" {
		t.Errorf("visitor value must win, got %v", out["interest"])
	}
}

func TestPrepareSubmittedData_MissingRequired(t *testing.T) {
	_, err := prepareSubmittedData(legacyStandardFields(), map[string]any{"name": "B", "email": "   ", "consent": true})
	var missing *ErrMissingRequired
	if !errors.As(err, &missing) {
		t.Fatalf("want *ErrMissingRequired, got %v", err)
	}
	if !reflect.DeepEqual(missing.Keys, []string{"email", "phone"}) {
		t.Errorf("keys = %v", missing.Keys)
	}
	if !strings.Contains(missing.Error(), "email,phone") {
		t.Errorf("Error() = %q", missing.Error())
	}
}

func TestPrepareSubmittedData_ConsentCheckboxMustBeTrue(t *testing.T) {
	for _, v := range []any{false, "true", nil} {
		_, err := prepareSubmittedData(legacyStandardFields(), map[string]any{"name": "B", "email": "b@x.id", "phone": "0812345678", "consent": v})
		var missing *ErrMissingRequired
		if !errors.As(err, &missing) || !reflect.DeepEqual(missing.Keys, []string{"consent"}) {
			t.Errorf("consent=%v: want missing [consent], got %v", v, err)
		}
	}
}

func TestPrepareSubmittedData_LegacyStandardFormPasses(t *testing.T) {
	// Payload persis seperti LeadFormSlot sebelum rilis ini.
	_, err := prepareSubmittedData(legacyStandardFields(), map[string]any{
		"name": "Budi", "email": "b@x.id", "phone": "0812345678", "company": "PT X",
		"company_size": "11–50", "interest": "Small Business", "message": "Halo",
		"consent": true, "page_url": "https://zyad.cloud/",
	})
	if err != nil {
		t.Fatalf("legacy form must pass: %v", err)
	}
}

func TestPrepareSubmittedData_NoFieldDefinitionsReturnsDataAsIs(t *testing.T) {
	in := map[string]any{"anything": "x"}
	out, err := prepareSubmittedData(nil, in)
	if err != nil || out["anything"] != "x" {
		t.Fatalf("out=%v err=%v", out, err)
	}
}

func TestSubmit_MissingRequiredCreatesNothing(t *testing.T) {
	f := newSyncFixture()
	f.forms.fields = legacyStandardFields()

	_, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), repository.CreateSubmissionParams{
		FormID: "form-1", IdempotencyKey: "k", SubmittedData: map[string]any{"name": "Budi"},
	}, true)
	var missing *ErrMissingRequired
	if !errors.As(err, &missing) {
		t.Fatalf("want *ErrMissingRequired, got %v", err)
	}
	if f.subs.createCalls != 0 || f.sink.calls != 0 {
		t.Fatalf("nothing may be created: create=%d sink=%d", f.subs.createCalls, f.sink.calls)
	}
}

func TestSubmit_LegacyFormWithoutCoreFields(t *testing.T) {
	f := newSyncFixture()
	f.forms.fields = []domain.LandingFormField{rulesField("message", domain.FormFieldTypeTextarea, false, 0, "")}

	if _, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), repository.CreateSubmissionParams{
		FormID: "form-1", IdempotencyKey: "k", SubmittedData: map[string]any{"message": "Halo"},
	}, true); err != nil {
		t.Fatalf("legacy form without core fields must still accept: %v", err)
	}
	if f.subs.createCalls != 1 {
		t.Fatalf("submission must be stored, createCalls=%d", f.subs.createCalls)
	}
}

func TestSubmit_HiddenDefaultReachesLeadNotes(t *testing.T) {
	f := newSyncFixture()
	fields := legacyStandardFields()
	fields[5] = rulesField("interest", domain.FormFieldTypeHidden, false, 5, "Paket 20 Mbps")
	fields[5].Label = "Minat"
	f.forms.fields = fields

	if _, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), repository.CreateSubmissionParams{
		FormID: "form-1", IdempotencyKey: "k",
		SubmittedData: map[string]any{"name": "Budi", "email": "b@x.id", "phone": "0812345678", "consent": true},
	}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.sink.input.Notes, "Minat: Paket 20 Mbps") {
		t.Fatalf("notes = %q", f.sink.input.Notes)
	}
}

func TestSubmit_HoneypotCheckedBeforeKeysAreDropped(t *testing.T) {
	f := newSyncFixture()
	f.forms.fields = legacyStandardFields()

	_, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), repository.CreateSubmissionParams{
		FormID: "form-1", IdempotencyKey: "k",
		SubmittedData: map[string]any{"name": "B", "email": "b@x.id", "phone": "0812345678", "consent": true, "_honey": "bot"},
	}, true)
	if !errors.Is(err, ErrSpamDetected) {
		t.Fatalf("want ErrSpamDetected, got %v", err)
	}
}
