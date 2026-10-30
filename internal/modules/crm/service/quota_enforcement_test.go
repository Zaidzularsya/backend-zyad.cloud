package service

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/organization"
)

// fakeRoom simulates QuotaEnforcer: it rejects once used reaches limit. The
// test bumps used itself after a successful create (the real meter counts rows).
type fakeRoom struct {
	limit int
	used  int
	keys  []string
	// fullKeys, when set, makes only those feature keys report "full" and
	// ignores limit/used (used by the lead-convert tests).
	fullKeys map[string]bool
}

func (f *fakeRoom) RequireRoom(_ context.Context, _ string, featureKey string) error {
	f.keys = append(f.keys, featureKey)
	if f.fullKeys != nil {
		if f.fullKeys[featureKey] {
			return organization.QuotaExceededError()
		}
		return nil
	}
	if f.used >= f.limit {
		return organization.QuotaExceededError()
	}
	return nil
}

func isQuotaExceeded(err error) bool {
	return err != nil && err.Error() == organization.QuotaExceededError().Error()
}

type quotaCompanyRepo struct {
	repository.CompanyRepository
	created, updated int
}

func (f *quotaCompanyRepo) Create(context.Context, coretenant.Scope, repository.CreateCompanyParams) (domain.Company, error) {
	f.created++
	return domain.Company{ID: "c"}, nil
}

func (f *quotaCompanyRepo) Update(_ context.Context, _ coretenant.Scope, id string, _ repository.UpdateCompanyParams) (domain.Company, error) {
	f.updated++
	return domain.Company{ID: id}, nil
}

type quotaDealRepo struct {
	repository.DealRepository
	created, updated int
}

func (f *quotaDealRepo) Create(context.Context, coretenant.Scope, repository.CreateDealParams) (domain.Deal, error) {
	f.created++
	return domain.Deal{ID: "d"}, nil
}

func (f *quotaDealRepo) Update(_ context.Context, _ coretenant.Scope, id string, _ repository.UpdateDealParams) (domain.Deal, error) {
	f.updated++
	return domain.Deal{ID: id}, nil
}

type quotaPipelineRepo struct {
	fakePipelineRepo
	active           int64
	created, updated int
}

func (f *quotaPipelineRepo) CountActive(context.Context, coretenant.Scope) (int64, error) {
	return f.active, nil
}

func (f *quotaPipelineRepo) Create(context.Context, coretenant.Scope, repository.CreatePipelineParams) (domain.Pipeline, error) {
	f.created++
	return domain.Pipeline{ID: "p"}, nil
}

func (f *quotaPipelineRepo) Update(_ context.Context, _ coretenant.Scope, id string, _ repository.UpdatePipelineParams) (domain.Pipeline, error) {
	f.updated++
	return domain.Pipeline{ID: id}, nil
}

// --- lead ---

func TestLeadCreateRejectedWhenQuotaFull(t *testing.T) {
	repo := &fakeLeadRepo{}
	room := &fakeRoom{limit: 0}
	svc := NewLeadService(repo, nil, nil, WithLeadQuotaEnforcer(room))

	_, err := svc.Create(context.Background(), testScope(t), repository.CreateLeadParams{ContactName: "Budi"})
	if !isQuotaExceeded(err) {
		t.Fatalf("Create() error = %v, want QUOTA_EXCEEDED", err)
	}
	if repo.lastCreate.ContactName != "" {
		t.Error("repository Create called despite full quota")
	}
	if len(room.keys) != 1 || room.keys[0] != domain.FeatureCRMMaxLeads {
		t.Errorf("enforcer keys = %v, want [%s]", room.keys, domain.FeatureCRMMaxLeads)
	}
}

func TestLeadCreateAllowedWhenRoom(t *testing.T) {
	repo := &fakeLeadRepo{}
	svc := NewLeadService(repo, nil, nil, WithLeadQuotaEnforcer(&fakeRoom{limit: 1}))

	if _, err := svc.Create(context.Background(), testScope(t), repository.CreateLeadParams{ContactName: "Budi"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repo.lastCreate.ContactName != "Budi" {
		t.Error("repository Create not called")
	}
}

func TestLeadCreateAtExactLimit(t *testing.T) {
	room := &fakeRoom{limit: 2}
	svc := NewLeadService(&fakeLeadRepo{}, nil, nil, WithLeadQuotaEnforcer(room))

	for i := 1; i <= 2; i++ {
		if _, err := svc.Create(context.Background(), testScope(t), repository.CreateLeadParams{ContactName: "L"}); err != nil {
			t.Fatalf("create #%d error = %v, want success", i, err)
		}
		room.used++
	}
	if _, err := svc.Create(context.Background(), testScope(t), repository.CreateLeadParams{ContactName: "L"}); !isQuotaExceeded(err) {
		t.Fatalf("create #3 error = %v, want QUOTA_EXCEEDED", err)
	}
}

func TestLeadCreateValidationRunsBeforeQuota(t *testing.T) {
	room := &fakeRoom{limit: 0}
	svc := NewLeadService(&fakeLeadRepo{}, nil, nil, WithLeadQuotaEnforcer(room))

	_, err := svc.Create(context.Background(), testScope(t), repository.CreateLeadParams{ContactName: "B", AnnualRevenue: "abc"})
	if err != ErrInvalidAnnualRevenue {
		t.Fatalf("Create() error = %v, want ErrInvalidAnnualRevenue", err)
	}
	if len(room.keys) != 0 {
		t.Error("enforcer consulted before validation")
	}
}

func TestLeadUpdateIgnoresQuota(t *testing.T) {
	room := &fakeRoom{limit: 0}
	svc := NewLeadService(&fakeLeadRepo{}, nil, nil, WithLeadQuotaEnforcer(room))
	name := "Butuh CRM"

	if _, err := svc.Update(context.Background(), testScope(t), "lead-1", repository.UpdateLeadParams{RequirementSummary: &name}); err != nil {
		t.Fatalf("Update() error = %v, want success despite full quota", err)
	}
	if len(room.keys) != 0 {
		t.Errorf("enforcer consulted on Update: %v", room.keys)
	}
}

// --- company ---

func TestCompanyCreateRejectedWhenQuotaFull(t *testing.T) {
	repo := &quotaCompanyRepo{}
	room := &fakeRoom{limit: 0}
	svc := NewCompanyService(repo, WithCompanyQuotaEnforcer(room))

	_, err := svc.Create(context.Background(), testScope(t), repository.CreateCompanyParams{Name: "PT A"})
	if !isQuotaExceeded(err) {
		t.Fatalf("Create() error = %v, want QUOTA_EXCEEDED", err)
	}
	if repo.created != 0 {
		t.Error("repository Create called despite full quota")
	}
	if len(room.keys) != 1 || room.keys[0] != domain.FeatureCRMMaxCompanies {
		t.Errorf("enforcer keys = %v", room.keys)
	}
}

func TestCompanyCreateAllowedWhenRoom(t *testing.T) {
	repo := &quotaCompanyRepo{}
	svc := NewCompanyService(repo, WithCompanyQuotaEnforcer(&fakeRoom{limit: 1}))
	if _, err := svc.Create(context.Background(), testScope(t), repository.CreateCompanyParams{Name: "PT A"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repo.created != 1 {
		t.Error("repository Create not called")
	}
}

func TestCompanyCreateAtExactLimit(t *testing.T) {
	room := &fakeRoom{limit: 2}
	svc := NewCompanyService(&quotaCompanyRepo{}, WithCompanyQuotaEnforcer(room))
	for i := 1; i <= 2; i++ {
		if _, err := svc.Create(context.Background(), testScope(t), repository.CreateCompanyParams{Name: "PT"}); err != nil {
			t.Fatalf("create #%d error = %v", i, err)
		}
		room.used++
	}
	if _, err := svc.Create(context.Background(), testScope(t), repository.CreateCompanyParams{Name: "PT"}); !isQuotaExceeded(err) {
		t.Fatalf("create #3 error = %v, want QUOTA_EXCEEDED", err)
	}
}

func TestCompanyUpdateIgnoresQuota(t *testing.T) {
	repo := &quotaCompanyRepo{}
	room := &fakeRoom{limit: 0}
	svc := NewCompanyService(repo, WithCompanyQuotaEnforcer(room))
	name := "Baru"
	if _, err := svc.Update(context.Background(), testScope(t), "c1", repository.UpdateCompanyParams{Name: &name}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(room.keys) != 0 || repo.updated != 1 {
		t.Errorf("enforcer keys=%v updated=%d", room.keys, repo.updated)
	}
}

// --- deal ---

func newQuotaDealService(room QuotaRoomRequirer) (DealService, *quotaDealRepo) {
	repo := &quotaDealRepo{}
	pipelines := &fakePipelineRepo{pipelines: map[string]domain.Pipeline{
		"p1": {ID: "p1", Stages: []domain.PipelineStage{{ID: "s1"}}},
	}}
	return NewDealService(repo, pipelines, WithDealQuotaEnforcer(room)), repo
}

func TestDealCreateRejectedWhenQuotaFull(t *testing.T) {
	room := &fakeRoom{limit: 0}
	svc, repo := newQuotaDealService(room)

	_, err := svc.Create(context.Background(), testScope(t), repository.CreateDealParams{PipelineID: "p1", StageID: "s1"})
	if !isQuotaExceeded(err) {
		t.Fatalf("Create() error = %v, want QUOTA_EXCEEDED", err)
	}
	if repo.created != 0 {
		t.Error("repository Create called despite full quota")
	}
	if len(room.keys) != 1 || room.keys[0] != domain.FeatureCRMMaxDeals {
		t.Errorf("enforcer keys = %v", room.keys)
	}
}

func TestDealCreateAllowedWhenRoom(t *testing.T) {
	svc, repo := newQuotaDealService(&fakeRoom{limit: 1})
	if _, err := svc.Create(context.Background(), testScope(t), repository.CreateDealParams{PipelineID: "p1", StageID: "s1"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repo.created != 1 {
		t.Error("repository Create not called")
	}
}

func TestDealCreateValidationRunsBeforeQuota(t *testing.T) {
	room := &fakeRoom{limit: 0}
	svc, _ := newQuotaDealService(room)
	_, err := svc.Create(context.Background(), testScope(t), repository.CreateDealParams{PipelineID: "p1", StageID: "nope"})
	if err != ErrDealStageNotInPipeline {
		t.Fatalf("Create() error = %v, want ErrDealStageNotInPipeline", err)
	}
	if len(room.keys) != 0 {
		t.Error("enforcer consulted before validation")
	}
}

func TestDealCreateAtExactLimit(t *testing.T) {
	room := &fakeRoom{limit: 2}
	svc, _ := newQuotaDealService(room)
	p := repository.CreateDealParams{PipelineID: "p1", StageID: "s1"}
	for i := 1; i <= 2; i++ {
		if _, err := svc.Create(context.Background(), testScope(t), p); err != nil {
			t.Fatalf("create #%d error = %v", i, err)
		}
		room.used++
	}
	if _, err := svc.Create(context.Background(), testScope(t), p); !isQuotaExceeded(err) {
		t.Fatalf("create #3 error = %v, want QUOTA_EXCEEDED", err)
	}
}

func TestDealUpdateIgnoresQuota(t *testing.T) {
	room := &fakeRoom{limit: 0}
	svc, _ := newQuotaDealService(room)
	title := "Baru"
	// Update may fail for unrelated reasons in the fake (stage lookup); the
	// contract under test is that the enforcer is never consulted.
	_, _ = svc.Update(context.Background(), testScope(t), "d1", repository.UpdateDealParams{Title: &title})
	if len(room.keys) != 0 {
		t.Errorf("enforcer consulted on Update: %v", room.keys)
	}
}

// --- pipeline ---

func TestPipelineCreateRejectedWhenQuotaFull(t *testing.T) {
	repo := &quotaPipelineRepo{active: 1}
	room := &fakeRoom{limit: 0}
	svc := NewPipelineService(repo, WithPipelineQuotaEnforcer(room))

	_, err := svc.Create(context.Background(), testScope(t), repository.CreatePipelineParams{Name: "P2"})
	if !isQuotaExceeded(err) {
		t.Fatalf("Create() error = %v, want QUOTA_EXCEEDED", err)
	}
	if repo.created != 0 {
		t.Error("repository Create called despite full quota")
	}
	if len(room.keys) != 1 || room.keys[0] != domain.FeatureCRMMaxPipelines {
		t.Errorf("enforcer keys = %v", room.keys)
	}
}

func TestPipelineFirstPipelineIsFreeEvenWhenQuotaFull(t *testing.T) {
	repo := &quotaPipelineRepo{active: 0}
	room := &fakeRoom{limit: 0}
	svc := NewPipelineService(repo, WithPipelineQuotaEnforcer(room))

	if _, err := svc.Create(context.Background(), testScope(t), repository.CreatePipelineParams{Name: "P1"}); err != nil {
		t.Fatalf("first pipeline error = %v, want success", err)
	}
	if len(room.keys) != 0 || repo.created != 1 {
		t.Errorf("keys=%v created=%d", room.keys, repo.created)
	}
}

func TestPipelineCreateAllowedWhenRoom(t *testing.T) {
	repo := &quotaPipelineRepo{active: 1}
	svc := NewPipelineService(repo, WithPipelineQuotaEnforcer(&fakeRoom{limit: 5}))
	if _, err := svc.Create(context.Background(), testScope(t), repository.CreatePipelineParams{Name: "P2"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repo.created != 1 {
		t.Error("repository Create not called")
	}
}

func TestPipelineCreateAtExactLimit(t *testing.T) {
	repo := &quotaPipelineRepo{active: 1}
	room := &fakeRoom{limit: 3, used: 1}
	svc := NewPipelineService(repo, WithPipelineQuotaEnforcer(room))
	for i := 1; i <= 2; i++ {
		if _, err := svc.Create(context.Background(), testScope(t), repository.CreatePipelineParams{Name: "P"}); err != nil {
			t.Fatalf("create #%d error = %v", i, err)
		}
		room.used++
		repo.active++
	}
	if _, err := svc.Create(context.Background(), testScope(t), repository.CreatePipelineParams{Name: "P"}); !isQuotaExceeded(err) {
		t.Fatalf("create #3 error = %v, want QUOTA_EXCEEDED", err)
	}
}

func TestPipelineUpdateIgnoresQuota(t *testing.T) {
	repo := &quotaPipelineRepo{active: 5}
	room := &fakeRoom{limit: 0}
	svc := NewPipelineService(repo, WithPipelineQuotaEnforcer(room))
	name := "Baru"
	if _, err := svc.Update(context.Background(), testScope(t), "p1", repository.UpdatePipelineParams{Name: &name}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(room.keys) != 0 || repo.updated != 1 {
		t.Errorf("keys=%v updated=%d", room.keys, repo.updated)
	}
}

// --- lead convert: creates company/deal rows, so it consumes those quotas ---

func convertQuotaFixture(room QuotaRoomRequirer) (LeadService, *fakeConvertLeadRepo) {
	repo := &fakeConvertLeadRepo{fakeLeadRepo: fakeLeadRepo{leads: map[string]domain.Lead{
		"l1": {ID: "l1", ContactName: "Budi", CompanyName: "PT Maju", Status: domain.LeadStatusQualified},
	}}}
	pipelines := &fakePipelineRepo{pipelines: map[string]domain.Pipeline{
		"p1": {ID: "p1", Stages: []domain.PipelineStage{{ID: "s1"}}},
	}}
	return NewLeadService(repo, nil, nil, WithLeadDealPipelines(pipelines), WithLeadQuotaEnforcer(room)), repo
}

func TestConvertNewCompanyRejectedWhenCompanyQuotaFull(t *testing.T) {
	svc, repo := convertQuotaFixture(&fakeRoom{fullKeys: map[string]bool{domain.FeatureCRMMaxCompanies: true}})
	_, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{
		Company: &ConvertCompanyInput{Mode: "new", Name: "PT Baru"},
	})
	if !isQuotaExceeded(err) {
		t.Fatalf("Convert() error = %v, want QUOTA_EXCEEDED", err)
	}
	if repo.lastConvert.Contact.FirstName != "" {
		t.Error("ConvertLead called despite full company quota")
	}
}

func TestConvertExistingOrNoCompanyIgnoresCompanyQuota(t *testing.T) {
	room := &fakeRoom{fullKeys: map[string]bool{domain.FeatureCRMMaxCompanies: true}}
	svc, _ := convertQuotaFixture(room)
	if _, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{Company: &ConvertCompanyInput{Mode: "none"}}); err != nil {
		t.Fatalf("Convert(none) error = %v", err)
	}
	if len(room.keys) != 0 {
		t.Errorf("enforcer consulted without a new company/deal: %v", room.keys)
	}
}

func TestConvertDealRejectedWhenDealQuotaFull(t *testing.T) {
	svc, repo := convertQuotaFixture(&fakeRoom{fullKeys: map[string]bool{domain.FeatureCRMMaxDeals: true}})
	_, err := svc.Convert(context.Background(), testScope(t), "l1", ConvertLeadParams{
		Deal: &ConvertDealInput{PipelineID: "p1", StageID: "s1", Title: "Internet"},
	})
	if !isQuotaExceeded(err) {
		t.Fatalf("Convert() error = %v, want QUOTA_EXCEEDED", err)
	}
	if repo.lastConvert.Contact.FirstName != "" {
		t.Error("ConvertLead called despite full deal quota")
	}
}
