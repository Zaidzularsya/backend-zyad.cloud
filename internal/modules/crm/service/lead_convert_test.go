// lead_convert_test.go
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type fakePipelineRepo struct {
	repository.PipelineRepository
	pipelines map[string]domain.Pipeline
}

func (f *fakePipelineRepo) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Pipeline, error) {
	p, ok := f.pipelines[id]
	if !ok {
		return domain.Pipeline{}, pgx.ErrNoRows
	}
	return p, nil
}

type fakeConvertLeadRepo struct {
	fakeLeadRepo
	lastConvert repository.ConvertLeadTxParams
}

func (f *fakeConvertLeadRepo) ConvertLead(_ context.Context, _ coretenant.Scope, id string, p repository.ConvertLeadTxParams) (domain.LeadConversionResult, error) {
	f.lastConvert = p
	return domain.LeadConversionResult{Lead: domain.Lead{ID: id, Status: domain.LeadStatusConverted}}, nil
}

func convertFixture() (*leadService, *fakeConvertLeadRepo) {
	archived := time.Now()
	repo := &fakeConvertLeadRepo{fakeLeadRepo: fakeLeadRepo{leads: map[string]domain.Lead{
		"l1": {ID: "l1", ContactName: "Budi", CompanyName: "PT Maju", Status: domain.LeadStatusQualified},
	}}}
	pipelines := &fakePipelineRepo{pipelines: map[string]domain.Pipeline{
		"p1":   {ID: "p1", Stages: []domain.PipelineStage{{ID: "s1", Position: 0}, {ID: "won", Position: 1, IsWon: true}}},
		"arch": {ID: "arch", ArchivedAt: &archived, Stages: []domain.PipelineStage{{ID: "s9"}}},
	}}
	svc := NewLeadService(repo, nil, nil, WithLeadDealPipelines(pipelines)).(*leadService)
	return svc, repo
}

func TestConvertValidatesDealStage(t *testing.T) {
	svc, repo := convertFixture()
	cases := []struct {
		name string
		deal ConvertDealInput
		want error
	}{
		{"unknown pipeline", ConvertDealInput{PipelineID: "nope", StageID: "s1", Title: "X"}, ErrInvalidPipelineStage},
		{"archived pipeline", ConvertDealInput{PipelineID: "arch", StageID: "s9", Title: "X"}, ErrInvalidPipelineStage},
		{"stage of another pipeline", ConvertDealInput{PipelineID: "p1", StageID: "s9", Title: "X"}, ErrInvalidPipelineStage},
		{"won stage", ConvertDealInput{PipelineID: "p1", StageID: "won", Title: "X"}, ErrInvalidStartStage},
		{"blank title", ConvertDealInput{PipelineID: "p1", StageID: "s1", Title: "  "}, ErrInvalidDealInput},
		{"bad value", ConvertDealInput{PipelineID: "p1", StageID: "s1", Title: "X", Value: "15.000.000"}, ErrInvalidDealInput},
		{"bad date", ConvertDealInput{PipelineID: "p1", StageID: "s1", Title: "X", ExpectedCloseDate: "30/11/2026"}, ErrInvalidDealInput},
	}
	for _, c := range cases {
		deal := c.deal
		_, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{Deal: &deal})
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if repo.lastConvert.Contact.FirstName != "" {
		t.Error("repository must not be called for invalid input")
	}
}

func TestConvertBuildsTxParams(t *testing.T) {
	svc, repo := convertFixture()
	_, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{
		Company: &ConvertCompanyInput{Mode: "new", Name: " PT Maju Jaya ", Industry: "ISP"},
		Deal: &ConvertDealInput{
			PipelineID: "p1", StageID: "s1", Title: " Internet ", Value: "",
			ExpectedCloseDate: "2026-11-30", Description: "50 Mbps", DecisionMaker: "Bu Rina",
		},
		OwnerUserID: "u1", ConvertedBy: "u1",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := repo.lastConvert
	if p.NewCompany == nil || p.NewCompany.Name != "PT Maju Jaya" || p.NewCompany.Industry != "ISP" {
		t.Fatalf("company = %+v", p.NewCompany)
	}
	if p.Contact.FirstName != "Budi" || p.Contact.OwnerUserID != "u1" {
		t.Fatalf("contact = %+v", p.Contact)
	}
	if p.Deal == nil || p.Deal.Title != "Internet" || p.Deal.Value != "0" || p.Deal.Description != "50 Mbps" ||
		p.Deal.ExpectedCloseDate == nil || p.Deal.ExpectedCloseDate.Format("2006-01-02") != "2026-11-30" ||
		p.Deal.OwnerUserID != "u1" {
		t.Fatalf("deal = %+v", p.Deal)
	}
}

func TestConvertLegacyCreateCompanyStillWorks(t *testing.T) {
	svc, repo := convertFixture()
	if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{CreateCompany: true}); err != nil {
		t.Fatal(err)
	}
	if repo.lastConvert.NewCompany == nil || repo.lastConvert.NewCompany.Name != "PT Maju" || repo.lastConvert.Deal != nil {
		t.Fatalf("legacy params = %+v", repo.lastConvert)
	}
}

func TestConvertCompanyModes(t *testing.T) {
	svc, repo := convertFixture()
	if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{Company: &ConvertCompanyInput{Mode: "none"}}); err != nil {
		t.Fatal(err)
	}
	if repo.lastConvert.NewCompany != nil || repo.lastConvert.ExistingCompanyID != "" {
		t.Fatal("mode none must not link a company")
	}
	for _, bad := range []ConvertCompanyInput{{Mode: "new", Name: " "}, {Mode: "existing"}, {Mode: "weird"}} {
		bad := bad
		if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{Company: &bad}); !errors.Is(err, ErrInvalidCompanyInput) {
			t.Errorf("%+v: err = %v", bad, err)
		}
	}
}

func TestConvertRejectsAlreadyConverted(t *testing.T) {
	svc, repo := convertFixture()
	repo.leads["l1"] = domain.Lead{ID: "l1", ContactName: "Budi", Status: domain.LeadStatusConverted}
	if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{}); !errors.Is(err, ErrLeadAlreadyConverted) {
		t.Fatalf("err = %v", err)
	}
}

func TestDealTitleFallback(t *testing.T) {
	if got := DefaultDealTitle(domain.Lead{ContactName: "Budi"}); got != "Deal Budi" {
		t.Errorf("no summary → %q", got)
	}
	long := domain.Lead{ContactName: "Budi", RequirementSummary: strings.Repeat("Internet dedicated 50 Mbps ", 10)}
	if got := DefaultDealTitle(long); len([]rune(got)) != 120 {
		t.Errorf("title too long: %d", len([]rune(got)))
	}
}

func TestConvertExistingContact(t *testing.T) {
	svc, repo := convertFixture()
	svc.contactRepo = &fakeContactRepo{contacts: map[string]domain.Contact{"c1": {ID: "c1"}}}

	if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{
		Contact: &ConvertContactInput{Mode: "existing", ContactID: "c1"},
	}); err != nil {
		t.Fatal(err)
	}
	if repo.lastConvert.ExistingContactID != "c1" {
		t.Fatalf("ExistingContactID = %q", repo.lastConvert.ExistingContactID)
	}

	// contact tidak ditemukan / bukan milik org → not found, repo tidak dipanggil
	repo.lastConvert = repository.ConvertLeadTxParams{}
	_, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{
		Contact: &ConvertContactInput{Mode: "existing", ContactID: "other-org"},
	})
	if err == nil || strings.Contains(err.Error(), "invalid") || repo.lastConvert.UpdatedBy != "" || repo.lastConvert.ExistingContactID != "" {
		t.Fatalf("unknown contact: err=%v params=%+v", err, repo.lastConvert)
	}

	for _, bad := range []ConvertContactInput{{Mode: "existing"}, {Mode: "weird"}} {
		bad := bad
		if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{Contact: &bad}); !errors.Is(err, ErrInvalidContactInput) {
			t.Errorf("%+v: err = %v", bad, err)
		}
	}
	// default / "new" tetap membuat contact baru
	repo.lastConvert = repository.ConvertLeadTxParams{}
	if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{Contact: &ConvertContactInput{Mode: "new"}}); err != nil {
		t.Fatal(err)
	}
	if repo.lastConvert.ExistingContactID != "" || repo.lastConvert.Contact.FirstName != "Budi" {
		t.Fatalf("new mode params = %+v", repo.lastConvert)
	}
}

func dedupeConvertFixture(existing ...domain.Contact) (*leadService, *fakeConvertLeadRepo) {
	repo := &fakeConvertLeadRepo{fakeLeadRepo: fakeLeadRepo{leads: map[string]domain.Lead{
		"l1": {ID: "l1", ContactName: "Budi", Email: " BUDI@b.com", Phone: "+62 812-3456-7890", Status: domain.LeadStatusQualified},
		"l2": {ID: "l2", ContactName: "Sari", Email: "sari@b.com", Status: domain.LeadStatusQualified},
	}}}
	contacts := &dedupeContactRepo{contacts: existing}
	svc := NewLeadService(repo, contacts, nil).(*leadService)
	return svc, repo
}

func TestMatchContacts_ByEmailAndPhone(t *testing.T) {
	svc, _ := dedupeConvertFixture(
		domain.Contact{ID: "by-email", Email: "budi@b.com"},
		domain.Contact{ID: "by-phone", Phone: "081234567890"},
		domain.Contact{ID: "other", Email: "x@b.com"},
	)
	got, err := svc.MatchContacts(context.Background(), testScope(t), "l1")
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, c := range got {
		ids = append(ids, c.ID)
	}
	if strings.Join(ids, ",") != "by-email,by-phone" {
		t.Fatalf("matches = %v", ids)
	}

	// One contact matching both email and phone is listed once.
	svc, _ = dedupeConvertFixture(domain.Contact{ID: "both", Email: "budi@b.com", Phone: "0812 3456 7890"})
	got, _ = svc.MatchContacts(context.Background(), testScope(t), "l1")
	if len(got) != 1 || got[0].ID != "both" {
		t.Fatalf("matches = %+v", got)
	}

	if _, err := svc.MatchContacts(context.Background(), testScope(t), "missing"); err == nil {
		t.Fatal("unknown lead must fail")
	}
}

func TestConvert_NewModeDuplicateIsRejected(t *testing.T) {
	svc, repo := dedupeConvertFixture(domain.Contact{ID: "old", Email: "budi@b.com"})

	_, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{})
	var dup *DuplicateContactError
	if !errors.As(err, &dup) || dup.Existing.ID != "old" {
		t.Fatalf("Convert(new) error = %v, want DuplicateContactError", err)
	}
	if repo.lastConvert.Contact.FirstName != "" {
		t.Fatal("convert must not run")
	}

	// mode existing with the matching contact still works.
	if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{
		Contact: &ConvertContactInput{Mode: "existing", ContactID: "old"},
	}); err != nil {
		t.Fatalf("Convert(existing) error = %v", err)
	}
	if repo.lastConvert.ExistingContactID != "old" {
		t.Fatalf("ExistingContactID = %q", repo.lastConvert.ExistingContactID)
	}

	// A lead without a match converts as before.
	if _, err := svc.Convert(context.Background(), testScope(t), "l2", ConvertLeadParams{}); err != nil {
		t.Fatalf("Convert(no match) error = %v", err)
	}
}

func TestConvert_AllowDuplicateContactKeepsSelfServeBehaviour(t *testing.T) {
	svc, repo := dedupeConvertFixture(domain.Contact{ID: "old", Email: "budi@b.com"})
	if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{AllowDuplicateContact: true}); err != nil {
		t.Fatalf("Convert(allow duplicate) error = %v", err)
	}
	if repo.lastConvert.Contact.FirstName != "Budi" {
		t.Fatal("contact should be created")
	}
}
