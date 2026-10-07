package service

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// LeadSourceLandingPage adalah Source lead yang berasal dari form landing page.
const LeadSourceLandingPage = "landing_page"

// FormLeadInput adalah data submission form landing yang siap dijadikan lead.
// OwnerUserID dipakai sebagai owner dan creator lead.
type FormLeadInput struct {
	SubmissionID, OwnerUserID, Name, Email, Phone, Company, Notes string
}

type FormLeadResult struct {
	LeadID string
	// Merged true: submission ditambahkan sebagai aktivitas pada lead terbuka
	// yang sudah ada, tidak ada lead baru.
	Merged bool
}

// FormLeadIntake membuat lead baru dari submission form, atau menempelkan
// submission sebagai note pada lead terbuka dengan email yang sama.
type FormLeadIntake interface {
	Intake(ctx context.Context, scope coretenant.Scope, in FormLeadInput) (FormLeadResult, error)
}

type formLeadIntake struct {
	leads      repository.LeadRepository
	leadSvc    LeadService
	activities ActivityService
}

func NewFormLeadIntake(leads repository.LeadRepository, leadSvc LeadService, activities ActivityService) FormLeadIntake {
	return &formLeadIntake{leads: leads, leadSvc: leadSvc, activities: activities}
}

func (f *formLeadIntake) Intake(ctx context.Context, scope coretenant.Scope, in FormLeadInput) (FormLeadResult, error) {
	email := strings.TrimSpace(in.Email)

	// Email kosong tidak bisa dedup: selalu lead baru.
	if email != "" {
		existing, err := f.leads.FindOpenByEmail(ctx, scope, email)
		switch {
		case err == nil:
			_, err := f.activities.Create(ctx, scope, repository.CreateActivityParams{
				RelatedEntityType: domain.ActivityEntityLead,
				RelatedEntityID:   existing.ID,
				Type:              domain.ActivityTypeNote,
				Status:            domain.ActivityStatusCompleted,
				Subject:           "Mengisi form lagi",
				Description:       in.Notes,
				CreatedBy:         in.OwnerUserID,
				Metadata:          map[string]any{"source": "landing_form", "submission_id": in.SubmissionID},
			})
			if err != nil {
				return FormLeadResult{}, err
			}
			return FormLeadResult{LeadID: existing.ID, Merged: true}, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return FormLeadResult{}, err
		}
	}

	lead, err := f.leadSvc.Create(ctx, scope, repository.CreateLeadParams{
		ContactName: in.Name,
		CompanyName: in.Company,
		Email:       email,
		Phone:       in.Phone,
		Source:      LeadSourceLandingPage,
		OwnerUserID: in.OwnerUserID,
		Notes:       in.Notes,
		CreatedBy:   in.OwnerUserID,
	})
	if err != nil {
		return FormLeadResult{}, err
	}
	return FormLeadResult{LeadID: lead.ID}, nil
}
