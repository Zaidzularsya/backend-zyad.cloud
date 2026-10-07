package app

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmservice "zyad.cloud/internal/modules/crm/service"
	landingservice "zyad.cloud/internal/modules/landing/service"
)

// landingLeadSink adapts crm's FormLeadIntake to landing's LeadSink so the
// landing module does not import crm.
type landingLeadSink struct {
	intake crmservice.FormLeadIntake
}

func (s landingLeadSink) CreateFromSubmission(ctx context.Context, scope coretenant.Scope, in landingservice.LeadSinkInput) (landingservice.LeadSinkResult, error) {
	res, err := s.intake.Intake(ctx, scope, crmservice.FormLeadInput{
		SubmissionID: in.SubmissionID,
		OwnerUserID:  in.OwnerUserID,
		Name:         in.Name,
		Email:        in.Email,
		Phone:        in.Phone,
		Company:      in.Company,
		Notes:        in.Notes,
	})
	if err != nil {
		return landingservice.LeadSinkResult{}, err
	}
	return landingservice.LeadSinkResult{LeadID: res.LeadID, Merged: res.Merged}, nil
}

var _ landingservice.LeadSink = landingLeadSink{}
var _ landingservice.FeatureChecker = entitlementChecker{}
