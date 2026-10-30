package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type grapesResolverRepoStub struct {
	repository.ResolverRepository
	forms []domain.LandingForm
}

func (s *grapesResolverRepoStub) ResolveForms(context.Context, coretenant.Scope, string) ([]domain.LandingForm, error) {
	return s.forms, nil
}
func (s *grapesResolverRepoStub) ResolveBranding(context.Context, coretenant.Scope, string) (domain.LandingBranding, error) {
	return domain.LandingBranding{ID: "b1"}, nil
}
func (s *grapesResolverRepoStub) ResolveVersions(context.Context, coretenant.Scope, string) ([]domain.LandingPageVersion, error) {
	return nil, nil
}
func (s *grapesResolverRepoStub) ResolveMenus(context.Context, coretenant.Scope) ([]domain.LandingMenu, error) {
	return nil, nil
}

type grapesDocumentRepoStub struct{ repository.DocumentRepository }

func (grapesDocumentRepoStub) GetByPageID(context.Context, coretenant.Scope, string) (domain.LandingPageDocument, error) {
	return domain.LandingPageDocument{}, pgx.ErrNoRows
}

type grapesFormRepoStub struct {
	repository.FormRepository
	fields map[string][]domain.LandingFormField
}

func (s *grapesFormRepoStub) ListFieldsByForm(_ context.Context, _ coretenant.Scope, formID string) ([]domain.LandingFormField, error) {
	return s.fields[formID], nil
}

func TestResolveGrapesPage_IncludesActiveForms(t *testing.T) {
	svc := &resolverService{
		resolverRepo: &grapesResolverRepoStub{forms: []domain.LandingForm{
			{ID: "f-active", Key: "konsultasi", IsActive: true},
			{ID: "f-off", Key: "lama", IsActive: false},
		}},
		formRepo: &grapesFormRepoStub{fields: map[string][]domain.LandingFormField{
			"f-active": {{Key: "email", Label: "Email kerja"}},
		}},
		documentRepo: grapesDocumentRepoStub{},
	}
	page := domain.LandingPage{ID: "p1", Builder: domain.PageBuilderGrapesJS}

	resolved := svc.resolveGrapesPage(context.Background(), mustLandingScope(t), page, true)

	if len(resolved.Forms) != 1 || resolved.Forms[0].ID != "f-active" {
		t.Fatalf("Forms = %+v, want only the active form", resolved.Forms)
	}
	if len(resolved.Forms[0].Fields) != 1 || resolved.Forms[0].Fields[0].Key != "email" {
		t.Fatalf("active form must carry its fields, got %+v", resolved.Forms[0].Fields)
	}
}

func TestResolveGrapesPage_HidesCRMConfigFromPublicForms(t *testing.T) {
	svc := &resolverService{
		resolverRepo: &grapesResolverRepoStub{forms: []domain.LandingForm{
			{ID: "f1", Key: "k", IsActive: true, CreateCRMLead: true, LeadOwnerUserID: "owner-uuid"},
		}},
		formRepo:     &grapesFormRepoStub{fields: map[string][]domain.LandingFormField{}},
		documentRepo: grapesDocumentRepoStub{},
	}
	page := domain.LandingPage{ID: "p1", Builder: domain.PageBuilderGrapesJS}
	resolved := svc.resolveGrapesPage(context.Background(), mustLandingScope(t), page, true)
	if len(resolved.Forms) != 1 {
		t.Fatalf("Forms = %+v", resolved.Forms)
	}
	if resolved.Forms[0].LeadOwnerUserID != "" || resolved.Forms[0].CreateCRMLead {
		t.Fatalf("public form leaks CRM config: %+v", resolved.Forms[0])
	}
}

func TestResolve_PublicFormsHideDefaultValue(t *testing.T) {
	svc := &resolverService{
		resolverRepo: &grapesResolverRepoStub{forms: []domain.LandingForm{{ID: "f1", Key: "k", IsActive: true}}},
		formRepo: &grapesFormRepoStub{fields: map[string][]domain.LandingFormField{
			"f1": {{Key: "interest", Type: domain.FormFieldTypeHidden, DefaultValue: "KODE-INTERNAL"}},
		}},
		documentRepo: grapesDocumentRepoStub{},
	}
	page := domain.LandingPage{ID: "p1", Builder: domain.PageBuilderGrapesJS}
	resolved := svc.resolveGrapesPage(context.Background(), mustLandingScope(t), page, true)

	if len(resolved.Forms) != 1 || len(resolved.Forms[0].Fields) != 1 {
		t.Fatalf("Forms = %+v", resolved.Forms)
	}
	if got := resolved.Forms[0].Fields[0].DefaultValue; got != "" {
		t.Fatalf("public form leaks default value %q", got)
	}
	raw, _ := json.Marshal(resolved)
	if strings.Contains(string(raw), "KODE-INTERNAL") {
		t.Fatalf("resolve JSON leaks default value: %s", raw)
	}
}

func TestResolve_PublicFormsIncludeAfterSubmit(t *testing.T) {
	svc := &resolverService{
		resolverRepo: &grapesResolverRepoStub{forms: []domain.LandingForm{{
			ID: "f1", Key: "k", IsActive: true,
			AfterSubmit: domain.AfterSubmitWhatsApp, WhatsAppNumber: "6281234567890", WhatsAppMessage: "Halo {nama}",
		}}},
		formRepo:     &grapesFormRepoStub{fields: map[string][]domain.LandingFormField{}},
		documentRepo: grapesDocumentRepoStub{},
	}
	page := domain.LandingPage{ID: "p1", Builder: domain.PageBuilderGrapesJS}
	resolved := svc.resolveGrapesPage(context.Background(), mustLandingScope(t), page, true)

	f := resolved.Forms[0]
	if f.AfterSubmit != domain.AfterSubmitWhatsApp || f.WhatsAppNumber != "6281234567890" || f.WhatsAppMessage != "Halo {nama}" {
		t.Fatalf("after-submit config must reach the public payload: %+v", f)
	}
}
