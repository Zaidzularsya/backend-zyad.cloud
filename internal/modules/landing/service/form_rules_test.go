package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

func TestNormalizeWhatsAppNumber(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"0812-3456-7890", "6281234567890", false},
		{"+62 812 3456 7890", "6281234567890", false},
		{"(021) 555 1234", "62215551234", false},
		{"0812345", "", true},
		{"08123456789012345678", "", true},
		{"08x1234567890", "", true},
		{"", "", false},
	}
	for _, tc := range cases {
		got, err := NormalizeWhatsAppNumber(tc.in)
		if tc.wantErr {
			if !errors.Is(err, ErrInvalidWhatsAppNumber) {
				t.Errorf("%q: want ErrInvalidWhatsAppNumber, got %v", tc.in, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%q: got %q err=%v, want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestNormalizeFieldParams_HiddenNeverRequired(t *testing.T) {
	out, err := normalizeFieldParams([]repository.CreateFormFieldParams{
		{Key: "interest", Type: domain.FormFieldTypeHidden, IsRequired: true, DefaultValue: " Paket 20 Mbps "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].IsRequired {
		t.Error("hidden field must not be required")
	}
	if out[0].DefaultValue != "Paket 20 Mbps" {
		t.Errorf("default value should be trimmed, got %q", out[0].DefaultValue)
	}
}

func TestNormalizeFieldParams_SelectDefaultMustBeOption(t *testing.T) {
	base := repository.CreateFormFieldParams{Key: "company_size", Type: domain.FormFieldTypeSelect, Options: []string{"1–10", "11–50"}}
	ok := base
	ok.DefaultValue = "11–50"
	if _, err := normalizeFieldParams([]repository.CreateFormFieldParams{ok}); err != nil {
		t.Errorf("valid option rejected: %v", err)
	}
	bad := base
	bad.DefaultValue = "99"
	if _, err := normalizeFieldParams([]repository.CreateFormFieldParams{bad}); !errors.Is(err, ErrInvalidDefaultValue) {
		t.Errorf("want ErrInvalidDefaultValue, got %v", err)
	}
	// Field select disembunyikan: tipe asli ada di validation.original_type, jadi opsi tetap berlaku.
	hidden := base
	hidden.Type = domain.FormFieldTypeHidden
	hidden.Validation = map[string]any{"original_type": "select"}
	hidden.DefaultValue = "99"
	if _, err := normalizeFieldParams([]repository.CreateFormFieldParams{hidden}); !errors.Is(err, ErrInvalidDefaultValue) {
		t.Errorf("hidden select with bad default: want ErrInvalidDefaultValue, got %v", err)
	}
}

func TestNormalizeFieldParams_DefaultTooLong(t *testing.T) {
	_, err := normalizeFieldParams([]repository.CreateFormFieldParams{
		{Key: "message", Type: domain.FormFieldTypeHidden, DefaultValue: strings.Repeat("a", 501)},
	})
	if !errors.Is(err, ErrInvalidDefaultValue) {
		t.Errorf("want ErrInvalidDefaultValue, got %v", err)
	}
}

func TestCheckCoreFields(t *testing.T) {
	text, email, phone := domain.FormFieldTypeText, domain.FormFieldTypeEmail, domain.FormFieldTypePhone
	full := []fieldShape{{"name", text, true}, {"email", email, true}, {"phone", phone, true}}
	with := func(i int, mutate func(*fieldShape)) []fieldShape {
		c := append([]fieldShape(nil), full...)
		mutate(&c[i])
		return c
	}
	cases := []struct {
		name   string
		crm    bool
		fields []fieldShape
		wantOK bool
	}{
		{"complete", true, full, true},
		{"email hidden", true, with(1, func(f *fieldShape) { f.Type = domain.FormFieldTypeHidden }), false},
		{"phone optional", true, with(2, func(f *fieldShape) { f.IsRequired = false }), false},
		{"name missing", true, full[1:], false},
		{"crm off, email missing", false, []fieldShape{full[0], full[2]}, true},
		{"no fields yet", true, nil, true},
	}
	for _, tc := range cases {
		err := checkCoreFields(tc.crm, tc.fields)
		if tc.wantOK && err != nil {
			t.Errorf("%s: unexpected %v", tc.name, err)
		}
		if !tc.wantOK && !errors.Is(err, ErrCoreFieldsRequired) {
			t.Errorf("%s: want ErrCoreFieldsRequired, got %v", tc.name, err)
		}
	}
}

func TestCheckAfterSubmit(t *testing.T) {
	cases := []struct {
		name         string
		action       domain.AfterSubmitAction
		redirect, wa string
		wantOK       bool
	}{
		{"message", domain.AfterSubmitMessage, "", "", true},
		{"redirect ok", domain.AfterSubmitRedirect, "/terima-kasih", "", true},
		{"redirect js", domain.AfterSubmitRedirect, "javascript:x", "", false},
		{"redirect protocol-relative", domain.AfterSubmitRedirect, "//evil", "", false},
		{"redirect empty", domain.AfterSubmitRedirect, "", "", false},
		{"whatsapp ok", domain.AfterSubmitWhatsApp, "", "6281234567890", true},
		{"whatsapp no number", domain.AfterSubmitWhatsApp, "", "", false},
		{"unknown", domain.AfterSubmitAction("popup"), "", "", false},
		{"empty", domain.AfterSubmitAction(""), "", "", false},
	}
	for _, tc := range cases {
		err := checkAfterSubmit(tc.action, tc.redirect, tc.wa)
		if tc.wantOK && err != nil {
			t.Errorf("%s: unexpected %v", tc.name, err)
		}
		if !tc.wantOK && !errors.Is(err, ErrInvalidAfterSubmit) {
			t.Errorf("%s: want ErrInvalidAfterSubmit, got %v", tc.name, err)
		}
	}
}

// fakeFormRepo: repository in-memory minimal untuk menguji urutan validasi di formService.
type fakeFormRepo struct {
	repository.FormRepository
	form    domain.LandingForm
	fields  []domain.LandingFormField
	updated *repository.UpdateFormParams
}

func (f *fakeFormRepo) FindByID(context.Context, coretenant.Scope, string) (domain.LandingForm, error) {
	return f.form, nil
}
func (f *fakeFormRepo) ListFieldsByForm(context.Context, coretenant.Scope, string) ([]domain.LandingFormField, error) {
	return append([]domain.LandingFormField(nil), f.fields...), nil
}
func (f *fakeFormRepo) DeleteField(_ context.Context, _ coretenant.Scope, id string) error {
	for i, fl := range f.fields {
		if fl.ID == id {
			f.fields = append(f.fields[:i], f.fields[i+1:]...)
			break
		}
	}
	return nil
}
func (f *fakeFormRepo) CreateField(_ context.Context, _ coretenant.Scope, p repository.CreateFormFieldParams) (domain.LandingFormField, error) {
	fl := domain.LandingFormField{ID: "n-" + p.Key, Key: p.Key, Type: p.Type, IsRequired: p.IsRequired, DefaultValue: p.DefaultValue}
	f.fields = append(f.fields, fl)
	return fl, nil
}
func (f *fakeFormRepo) Update(_ context.Context, _ coretenant.Scope, _ string, p repository.UpdateFormParams) (domain.LandingForm, error) {
	f.updated = &p
	return f.form, nil
}

func coreFieldRows() []domain.LandingFormField {
	return []domain.LandingFormField{
		{ID: "1", Key: "name", Type: domain.FormFieldTypeText, IsRequired: true},
		{ID: "2", Key: "email", Type: domain.FormFieldTypeEmail, IsRequired: true},
		{ID: "3", Key: "phone", Type: domain.FormFieldTypePhone, IsRequired: true},
		{ID: "4", Key: "message", Type: domain.FormFieldTypeTextarea},
	}
}

func TestReplaceFields_RejectKeepsExistingFields(t *testing.T) {
	repo := &fakeFormRepo{form: domain.LandingForm{ID: "f", CreateCRMLead: true}, fields: coreFieldRows()}
	svc := NewFormService(repo)

	_, err := svc.ReplaceFields(context.Background(), coretenant.Scope{}, "f", []repository.CreateFormFieldParams{
		{Key: "name", Type: domain.FormFieldTypeText, IsRequired: true},
		{Key: "phone", Type: domain.FormFieldTypePhone, IsRequired: true},
	})
	if !errors.Is(err, ErrCoreFieldsRequired) {
		t.Fatalf("want ErrCoreFieldsRequired, got %v", err)
	}
	if len(repo.fields) != 4 {
		t.Fatalf("existing fields must stay intact, got %d", len(repo.fields))
	}
}

func TestUpdateForm_EnablingCRMRequiresCoreFields(t *testing.T) {
	repo := &fakeFormRepo{
		form:   domain.LandingForm{ID: "f", CreateCRMLead: false, AfterSubmit: domain.AfterSubmitMessage},
		fields: coreFieldRows()[:2], // tanpa phone
	}
	yes := true
	_, err := NewFormService(repo).UpdateForm(context.Background(), coretenant.Scope{}, "f", repository.UpdateFormParams{CreateCRMLead: &yes})
	if !errors.Is(err, ErrCoreFieldsRequired) {
		t.Fatalf("want ErrCoreFieldsRequired, got %v", err)
	}
	if repo.updated != nil {
		t.Error("repository.Update must not be called when validation fails")
	}
}

func TestUpdateForm_NormalizesWhatsAppNumber(t *testing.T) {
	repo := &fakeFormRepo{form: domain.LandingForm{ID: "f", AfterSubmit: domain.AfterSubmitMessage}}
	num := "0812 3456 7890"
	action := domain.AfterSubmitWhatsApp
	if _, err := NewFormService(repo).UpdateForm(context.Background(), coretenant.Scope{}, "f", repository.UpdateFormParams{
		AfterSubmit: &action, WhatsAppNumber: &num,
	}); err != nil {
		t.Fatal(err)
	}
	if repo.updated == nil || repo.updated.WhatsAppNumber == nil || *repo.updated.WhatsAppNumber != "6281234567890" {
		t.Fatalf("number not normalized: %+v", repo.updated)
	}
}

func TestUpdateForm_UnrelatedPatchSkipsLegacyChecks(t *testing.T) {
	// Form lama dengan data yang tidak lolos aturan baru tetap bisa diubah hal lain (mis. is_active).
	repo := &fakeFormRepo{
		form:   domain.LandingForm{ID: "f", CreateCRMLead: true, AfterSubmit: domain.AfterSubmitMessage},
		fields: coreFieldRows()[:1],
	}
	off := false
	if _, err := NewFormService(repo).UpdateForm(context.Background(), coretenant.Scope{}, "f", repository.UpdateFormParams{IsActive: &off}); err != nil {
		t.Fatalf("unrelated patch must pass: %v", err)
	}
}

func TestCreateForm_ValidatesAfterSubmitAndNormalizesWhatsApp(t *testing.T) {
	repo := &fakeFormRepo{}
	svc := NewFormService(repo)
	if _, err := svc.CreateForm(context.Background(), coretenant.Scope{}, repository.CreateFormParams{AfterSubmit: domain.AfterSubmitWhatsApp}); !errors.Is(err, ErrInvalidAfterSubmit) {
		t.Fatalf("whatsapp without number: want ErrInvalidAfterSubmit, got %v", err)
	}
}
