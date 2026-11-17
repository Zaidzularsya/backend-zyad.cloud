package service

import (
	"context"
	"fmt"
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
	// Control and Policy make the candidate rule match the evaluator: the
	// tenant's grace days shift the cutoff and an effectively exempt account
	// has no candidates. Both are required (the evaluator itself is nil while
	// the engine is off, so it cannot be used here).
	Control PublicControlReader
	Policy  PublicPolicyReader
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
// phone number (a phone-shaped query is tried both ways). Username matching (case/space-insensitive) is the
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

	accounts, err := s.findAccounts(ctx, scope, q)
	if err != nil {
		return PublicLookupResult{}, err
	}
	if len(accounts) != 1 {
		return PublicLookupResult{}, nil
	}
	acc := accounts[0]

	invoices, err := s.candidates(ctx, scope, acc.ID, today)
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

// findAccounts matches the query as a secret username and, when it is
// phone-shaped, also as a phone: PPPoE usernames are often all digits. The
// union is deduplicated by account id so the same account matched both ways
// counts once; two different accounts stay ambiguous.
func (s *PublicService) findAccounts(ctx context.Context, scope coretenant.Scope, q string) ([]AccountRef, error) {
	accounts, err := s.d.Directory.AccountsByUsername(ctx, scope, q)
	if err != nil {
		return nil, err
	}
	phone := phoneQuery(q)
	if phone == "" {
		return accounts, nil
	}
	byPhone, err := s.d.Directory.AccountsByPhone(ctx, scope, phone)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		seen[a.ID] = true
	}
	for _, a := range byPhone {
		if !seen[a.ID] {
			seen[a.ID] = true
			accounts = append(accounts, a)
		}
	}
	return accounts, nil
}

// candidates applies the evaluator's candidate rule (Evaluator.decide): an
// effectively exempt account has none, otherwise the tenant's grace days
// shift the cutoff passed to OverdueSource.
func (s *PublicService) candidates(ctx context.Context, scope coretenant.Scope, accountID string, today time.Time) ([]TriggerInvoice, error) {
	policy, _, err := s.d.Policy.GetCustomerPolicy(ctx, scope, accountID)
	if err != nil {
		return nil, fmt.Errorf("load customer policy: %w", err)
	}
	if EffectiveExempt(policy, today) {
		return nil, nil
	}
	ctl, err := s.d.Control.Get(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("load isolation control: %w", err)
	}
	return s.d.Overdue.TriggerInvoices(ctx, scope, accountID, CandidateCutoff(today, ctl.GraceDays))
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

// phoneQuery returns the normalized phone when q is a phone-shaped query, else "".
func phoneQuery(q string) string {
	if !looksLikePhone(q) {
		return ""
	}
	return NormalizePhone(q)
}

// LookupKey is the canonical form of a public lookup query, for rate-limit
// bucketing: phone formats of the same number collapse to one value, anything
// else is the trimmed lower-cased query. Same phone rule as Lookup.
func LookupKey(q string) string {
	q = strings.TrimSpace(q)
	if p := phoneQuery(q); p != "" {
		return p
	}
	return strings.ToLower(q)
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
