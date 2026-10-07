package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type intakeLeadRepo struct {
	repository.LeadRepository
	found     domain.Lead
	findErr   error
	findCalls int
	lastEmail string
}

func (r *intakeLeadRepo) FindOpenByEmail(_ context.Context, _ coretenant.Scope, email string) (domain.Lead, error) {
	r.findCalls++
	r.lastEmail = email
	return r.found, r.findErr
}

type intakeLeadSvc struct {
	LeadService
	created []repository.CreateLeadParams
	err     error
}

func (s *intakeLeadSvc) Create(_ context.Context, _ coretenant.Scope, p repository.CreateLeadParams) (domain.Lead, error) {
	s.created = append(s.created, p)
	if s.err != nil {
		return domain.Lead{}, s.err
	}
	return domain.Lead{ID: "new-lead"}, nil
}

type intakeActivitySvc struct {
	ActivityService
	created []repository.CreateActivityParams
	err     error
}

func (s *intakeActivitySvc) Create(_ context.Context, _ coretenant.Scope, p repository.CreateActivityParams) (domain.Activity, error) {
	s.created = append(s.created, p)
	return domain.Activity{ID: "act-1"}, s.err
}

func intakeInput() FormLeadInput {
	return FormLeadInput{
		SubmissionID: "sub-1", OwnerUserID: "owner-1", Name: "Budi",
		Email: "  Budi@X.id ", Phone: "0812", Company: "PT X", Notes: "Pesan: halo",
	}
}

func TestIntake_MergesIntoOpenLeadByEmail(t *testing.T) {
	repo := &intakeLeadRepo{found: domain.Lead{ID: "L1"}}
	leads, acts := &intakeLeadSvc{}, &intakeActivitySvc{}
	res, err := NewFormLeadIntake(repo, leads, acts).Intake(context.Background(), testScope(t), intakeInput())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Merged || res.LeadID != "L1" {
		t.Fatalf("result = %+v", res)
	}
	if repo.lastEmail != "Budi@X.id" {
		t.Fatalf("email must be trimmed, got %q", repo.lastEmail)
	}
	if len(leads.created) != 0 {
		t.Fatal("leadSvc.Create must not be called when merging")
	}
	if len(acts.created) != 1 {
		t.Fatalf("activities = %d", len(acts.created))
	}
	a := acts.created[0]
	if a.Type != domain.ActivityTypeNote || a.Status != domain.ActivityStatusCompleted ||
		a.RelatedEntityType != domain.ActivityEntityLead || a.RelatedEntityID != "L1" ||
		a.Subject != "Mengisi form lagi" || a.Description != "Pesan: halo" || a.CreatedBy != "owner-1" {
		t.Fatalf("activity = %+v", a)
	}
	if a.Metadata["submission_id"] != "sub-1" || a.Metadata["source"] != "landing_form" {
		t.Fatalf("metadata = %+v", a.Metadata)
	}
}

func TestIntake_CreatesLeadWhenNoOpenLead(t *testing.T) {
	repo := &intakeLeadRepo{findErr: pgx.ErrNoRows}
	leads, acts := &intakeLeadSvc{}, &intakeActivitySvc{}
	res, err := NewFormLeadIntake(repo, leads, acts).Intake(context.Background(), testScope(t), intakeInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged || res.LeadID != "new-lead" {
		t.Fatalf("result = %+v", res)
	}
	if len(acts.created) != 0 || len(leads.created) != 1 {
		t.Fatalf("activities=%d leads=%d", len(acts.created), len(leads.created))
	}
	p := leads.created[0]
	if p.Source != "landing_page" || p.SkipPlaybook || p.OwnerUserID != "owner-1" || p.CreatedBy != "owner-1" ||
		p.Notes != "Pesan: halo" || p.ContactName != "Budi" || p.Email != "Budi@X.id" || p.Phone != "0812" || p.CompanyName != "PT X" {
		t.Fatalf("params = %+v", p)
	}
}

func TestIntake_EmptyEmailSkipsDedup(t *testing.T) {
	repo := &intakeLeadRepo{found: domain.Lead{ID: "L1"}}
	leads := &intakeLeadSvc{}
	in := intakeInput()
	in.Email = "   "
	res, err := NewFormLeadIntake(repo, leads, &intakeActivitySvc{}).Intake(context.Background(), testScope(t), in)
	if err != nil {
		t.Fatal(err)
	}
	if repo.findCalls != 0 || res.Merged || len(leads.created) != 1 {
		t.Fatalf("findCalls=%d res=%+v created=%d", repo.findCalls, res, len(leads.created))
	}
}

func TestIntake_PropagatesCreateError(t *testing.T) {
	boom := errors.New("boom")
	repo := &intakeLeadRepo{findErr: pgx.ErrNoRows}
	_, err := NewFormLeadIntake(repo, &intakeLeadSvc{err: boom}, &intakeActivitySvc{}).Intake(context.Background(), testScope(t), intakeInput())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	// Error aktivitas pada merge juga diteruskan.
	repo = &intakeLeadRepo{found: domain.Lead{ID: "L1"}}
	_, err = NewFormLeadIntake(repo, &intakeLeadSvc{}, &intakeActivitySvc{err: boom}).Intake(context.Background(), testScope(t), intakeInput())
	if !errors.Is(err, boom) {
		t.Fatalf("activity err = %v", err)
	}
	// Error lookup selain ErrNoRows diteruskan, bukan dianggap "tidak ada".
	repo = &intakeLeadRepo{findErr: boom}
	leads := &intakeLeadSvc{}
	_, err = NewFormLeadIntake(repo, leads, &intakeActivitySvc{}).Intake(context.Background(), testScope(t), intakeInput())
	if !errors.Is(err, boom) || len(leads.created) != 0 {
		t.Fatalf("lookup err = %v created=%d", err, len(leads.created))
	}
}
