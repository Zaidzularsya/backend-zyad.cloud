package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

// FeatureCRMLeadForm is the entitlement key a tenant needs before its landing
// forms may create CRM leads. Platform organizations are exempt.
const FeatureCRMLeadForm = "crm.lead_form"

// leadOwnerMissingMessage is stored in crm_sync_error when neither the form nor
// the page has a PIC. The code prefix lets the UI recognise it.
const leadOwnerMissingMessage = "LANDING_FORM_OWNER_MISSING: PIC lead belum diatur"

// ErrLeadOwnerMissing means no lead owner could be derived for a submission.
var ErrLeadOwnerMissing = errors.New(leadOwnerMissingMessage)

// LeadSinkInput is a submission reduced to what a CRM lead needs.
type LeadSinkInput struct {
	SubmissionID, OwnerUserID, Name, Email, Phone, Company, Notes string
}

type LeadSinkResult struct {
	LeadID string
	// Merged is true when the submission was attached to an existing open lead.
	Merged bool
}

// LeadSink is implemented outside this module (internal/app) so that landing
// does not import crm.
type LeadSink interface {
	CreateFromSubmission(ctx context.Context, scope coretenant.Scope, in LeadSinkInput) (LeadSinkResult, error)
}

// FeatureChecker reports whether an organization is entitled to a feature; a
// nil error means entitled.
type FeatureChecker interface {
	RequireFeature(ctx context.Context, organizationID, featureKey string) error
}

// fields already mapped to dedicated lead columns, or not meant for the
// sales team; they are left out of the notes.
var leadNotesExcludedKeys = map[string]bool{
	"name": true, "email": true, "phone": true, "company": true, "consent": true, "website": true,
}

// buildLeadNotes renders one "<Label>: <value>" line per form field (form
// order), followed by the page URL and UTM values. Empty values are skipped.
func buildLeadNotes(form domain.LandingForm, sub domain.LandingSubmission) string {
	var lines []string
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			lines = append(lines, label+": "+value)
		}
	}

	for _, field := range form.Fields {
		if leadNotesExcludedKeys[field.Key] {
			continue
		}
		label := strings.TrimSpace(field.Label)
		if label == "" {
			label = field.Key
		}
		add(label, formatSubmittedValue(sub.SubmittedData[field.Key]))
	}

	pageURL := sub.SourceURL
	if pageURL == "" {
		pageURL = formatSubmittedValue(sub.SubmittedData["page_url"])
	}
	add("Halaman", pageURL)
	add("UTM source", sub.UTMSource)
	add("UTM medium", sub.UTMMedium)
	add("UTM campaign", sub.UTMCampaign)
	add("UTM term", sub.UTMTerm)
	add("UTM content", sub.UTMContent)

	return strings.Join(lines, "\n")
}

func formatSubmittedValue(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case []any:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			if s := strings.TrimSpace(formatSubmittedValue(item)); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(val)
	}
}

func submittedString(sub domain.LandingSubmission, key string) string {
	return strings.TrimSpace(formatSubmittedValue(sub.SubmittedData[key]))
}

// buildLeadSinkInput maps a submission to a lead. The owner is the form's PIC,
// falling back to the page creator; ErrLeadOwnerMissing if both are empty.
func buildLeadSinkInput(form domain.LandingForm, page domain.LandingPage, sub domain.LandingSubmission) (LeadSinkInput, error) {
	owner := strings.TrimSpace(form.LeadOwnerUserID)
	if owner == "" {
		owner = strings.TrimSpace(page.CreatedBy)
	}
	if owner == "" {
		return LeadSinkInput{}, ErrLeadOwnerMissing
	}

	return LeadSinkInput{
		SubmissionID: sub.ID,
		OwnerUserID:  owner,
		Name:         submittedString(sub, "name"),
		Email:        submittedString(sub, "email"),
		Phone:        submittedString(sub, "phone"),
		Company:      submittedString(sub, "company"),
		Notes:        buildLeadNotes(form, sub),
	}, nil
}
