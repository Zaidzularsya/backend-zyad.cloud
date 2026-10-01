package service

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type fakeDealRepo struct {
	repository.DealRepository
	deal domain.Deal
}

func (f *fakeDealRepo) FindByID(context.Context, coretenant.Scope, string) (domain.Deal, error) {
	return f.deal, nil
}

type fakeSourceLeadRepo struct {
	fakeLeadRepo
	src *domain.DealSourceLead
}

func (f *fakeSourceLeadRepo) FindSourceLeadByDealID(context.Context, coretenant.Scope, string) (*domain.DealSourceLead, error) {
	return f.src, nil
}

func TestDealGetDetailIncludesPipelineAndSourceLead(t *testing.T) {
	deals := &fakeDealRepo{deal: domain.Deal{ID: "d1", PipelineID: "p1", StageID: "s1"}}
	pipelines := &fakePipelineRepo{pipelines: map[string]domain.Pipeline{"p1": {ID: "p1", Name: "Utama"}}}
	leads := &fakeSourceLeadRepo{src: &domain.DealSourceLead{ID: "l1", ContactName: "Budi"}}
	svc := NewDealService(deals, pipelines, WithDealSourceLeads(leads))

	got, err := svc.GetDetail(context.Background(), testScope(t), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Deal.ID != "d1" || got.Pipeline.Name != "Utama" || got.SourceLead == nil || got.SourceLead.ContactName != "Budi" {
		t.Fatalf("detail = %+v", got)
	}
}
