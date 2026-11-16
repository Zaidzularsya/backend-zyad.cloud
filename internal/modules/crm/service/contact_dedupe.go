package service

import (
	"context"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/shared/phone"
)

// CodeContactDuplicate is returned (HTTP 409) when a contact would share its
// email or WhatsApp number with another contact of the same organization.
const CodeContactDuplicate = "CONTACT_DUPLICATE"

// NormalizePhone returns the WhatsApp matching form of a phone number
// (0812…/+62 812… → 62812…), or "" when it is not a plausible number. It
// delegates to phone.NormalizeID so it matches crm_contacts.phone_normalized.
func NormalizePhone(raw string) string {
	normalized, ok := phone.NormalizeID(raw)
	if !ok {
		return ""
	}
	return normalized
}

// NormalizeEmail trims and lower-cases an email for duplicate comparison.
func NormalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// DuplicateContactError carries the contact that already owns the email or
// phone, so the caller can offer it instead of creating a second one. It
// unwraps to a 409 AppError so plain corehttp.Fail still answers correctly.
type DuplicateContactError struct {
	Existing domain.Contact
	// Field is "email" or "phone": which value collided.
	Field string
}

func (e *DuplicateContactError) Error() string {
	return "contact with the same " + e.Field + " already exists"
}

func (e *DuplicateContactError) Unwrap() error {
	return coreerrors.New(CodeContactDuplicate, "Kontak dengan email atau nomor WhatsApp yang sama sudah ada.", http.StatusConflict)
}

// duplicateFinder is the slice of ContactRepository the guard needs; lead
// convert and the mikrotik onboarding reuse it.
type duplicateFinder interface {
	FindDuplicate(ctx context.Context, scope coretenant.Scope, email, phoneNormalized, excludeID string) (domain.Contact, bool, error)
}

// checkDuplicate rejects email/phone that another live contact of the
// organization already uses. Empty values are not compared. Email is checked
// before phone so the reported field is deterministic.
func checkDuplicate(ctx context.Context, repo duplicateFinder, scope coretenant.Scope, email, rawPhone, excludeID string) error {
	normalizedEmail := NormalizeEmail(email)
	if normalizedEmail != "" {
		existing, found, err := repo.FindDuplicate(ctx, scope, normalizedEmail, "", excludeID)
		if err != nil {
			return err
		}
		if found {
			return &DuplicateContactError{Existing: existing, Field: "email"}
		}
	}
	normalizedPhone := NormalizePhone(rawPhone)
	if normalizedPhone != "" {
		existing, found, err := repo.FindDuplicate(ctx, scope, "", normalizedPhone, excludeID)
		if err != nil {
			return err
		}
		if found {
			return &DuplicateContactError{Existing: existing, Field: "phone"}
		}
	}
	return nil
}
