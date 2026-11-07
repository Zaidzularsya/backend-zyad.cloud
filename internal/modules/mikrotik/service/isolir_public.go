package service

import (
	"context"
	"strings"
	"time"
	"unicode"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

const publicMaxQueryLen = 100

// PublicInfo tells whether the public isolir page is on for a tenant.
// Tenant is nil when !Enabled.
type PublicInfo struct {
	Enabled bool
	Tenant  *TenantContact
}

// PublicInvoiceItem is one overdue invoice shown to the customer.
// PayURL is "" when the tenant has no payment link (no default sender).
type PublicInvoiceItem struct {
	Number  string
	DueDate time.Time
	Balance string
	PayURL  string
}

// PublicLookupResult is the only thing the public page learns about a
// customer. The zero value is the uniform answer for every "nothing to show"
// case (no match, no debt, ambiguous, feature off, junk query) so callers
// cannot probe which accounts exist.
type PublicLookupResult struct {
	Found      bool
	MaskedName string
	Isolated   bool // at least one secret of the account is 'isolated'
	Invoices   []PublicInvoiceItem
}

// PublicServiceDeps are the collaborators of PublicService.
type PublicServiceDeps struct {
	Directory PublicDirectory
	Secrets   PublicSecrets
	Overdue   OverdueSource
	PayLinks  PublicPayLinks
	Profile   PublicTenantProfile
	Gate      PublicFeatureGate
	Tasks     RestoreTaskReader
}

// PublicService backs the unauthenticated isolir page.
type PublicService struct{ d PublicServiceDeps }

func NewPublicService(d PublicServiceDeps) *PublicService { return &PublicService{d: d} }

// Info returns the page state; Tenant is only filled when the feature is enabled.
func (s *PublicService) Info(ctx context.Context, scope coretenant.Scope) (PublicInfo, error) {
	on, err := s.d.Gate.Enabled(ctx, scope)
	if err != nil {
		return PublicInfo{}, err
	}
	if !on {
		return PublicInfo{}, nil
	}
	c, err := s.d.Profile.Profile(ctx, scope)
	if err != nil {
		return PublicInfo{}, err
	}
	return PublicInfo{Enabled: true, Tenant: &c}, nil
}

// Lookup finds the debt of the customer identified by a secret username or a
// phone number. Username matching (case/space-insensitive) is the
// PublicDirectory's job; the service passes the trimmed query. Anything that
// is not exactly one account with candidate invoices yields the zero result.
func (s *PublicService) Lookup(ctx context.Context, scope coretenant.Scope, query string, today time.Time) (PublicLookupResult, error) {
	q := strings.TrimSpace(query)
	if q == "" || len(q) > publicMaxQueryLen || hasControlChar(q) {
		return PublicLookupResult{}, nil
	}
	on, err := s.d.Gate.Enabled(ctx, scope)
	if err != nil {
		return PublicLookupResult{}, err
	}
	if !on {
		return PublicLookupResult{}, nil
	}

	var accounts []AccountRef
	if phone := NormalizePhone(q); phone != "" && looksLikePhone(q) {
		accounts, err = s.d.Directory.AccountsByPhone(ctx, scope, phone)
	} else {
		accounts, err = s.d.Directory.AccountsByUsername(ctx, scope, q)
	}
	if err != nil {
		return PublicLookupResult{}, err
	}
	if len(accounts) != 1 {
		return PublicLookupResult{}, nil
	}
	acc := accounts[0]

	invoices, err := s.d.Overdue.TriggerInvoices(ctx, scope, acc.ID, today)
	if err != nil {
		return PublicLookupResult{}, err
	}
	if len(invoices) == 0 {
		return PublicLookupResult{}, nil
	}
	secrets, err := s.d.Secrets.ListByAccount(ctx, scope, acc.ID)
	if err != nil {
		return PublicLookupResult{}, err
	}

	res := PublicLookupResult{Found: true, MaskedName: MaskName(acc.Name)}
	for _, sec := range secrets {
		if sec.IsolationState == domain.IsolationIsolated {
			res.Isolated = true
			break
		}
	}
	for _, inv := range invoices {
		url, err := s.d.PayLinks.PayLink(ctx, scope, inv.ID)
		if err != nil {
			return PublicLookupResult{}, err
		}
		res.Invoices = append(res.Invoices, PublicInvoiceItem{Number: inv.Number, DueDate: inv.DueDate, Balance: inv.Balance, PayURL: url})
	}
	return res, nil
}

// MaskName keeps the first letter of each word and hides the rest
// ("Budi Santoso" -> "B*** S******"). Blank names become "Pelanggan".
func MaskName(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return "Pelanggan"
	}
	for i, w := range words {
		r := []rune(w)
		words[i] = string(r[0]) + strings.Repeat("*", len(r)-1)
	}
	return strings.Join(words, " ")
}

func hasControlChar(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// looksLikePhone is true when s has only digits, + - ( ) and spaces.
func looksLikePhone(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r == '+', r == '-', r == '(', r == ')', r == ' ':
		default:
			return false
		}
	}
	return true
}
