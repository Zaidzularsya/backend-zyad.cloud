package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// OnboardingReader is satisfied by *repository.OnboardingRepository.
type OnboardingReader interface {
	Counts(ctx context.Context, scope coretenant.Scope) (repository.OnboardingCounts, error)
	AccountStatuses(ctx context.Context, scope coretenant.Scope, accountIDs []string) (map[string]repository.AccountStatus, error)
}

// OnboardingSummary is the setup checklist of the isolir feature.
type OnboardingSummary struct {
	RouterRegistered    bool // at least one non-deleted router
	RouterReady         bool // at least one router with isolir_ready
	SecretsTotal        int  // secrets still present on a router
	SecretsPaired       int  // present secrets linked to a customer account
	SecretsNeedsProfile int  // present secrets still waiting for an original profile
	ProductsMarked      int  // products with an enabled isolir trigger
	SenderConfigured    bool // receivable default sender is set
	Complete            bool
}

// Isolation states reported per customer.
const (
	CustomerNormal   = "normal"
	CustomerIsolated = "isolated"
	CustomerNone     = "none"
)

// CustomerStatus is the isolir status of one customer account.
type CustomerStatus struct {
	AccountID      string
	IsolationState string
	Exempt         bool
	Secrets        int
	// ExemptedUntil is "YYYY-MM-DD" or "" when the exemption has no expiry.
	ExemptedUntil    string
	PendingIsolation bool
}

type OnboardingService struct {
	repo   OnboardingReader
	sender SenderSettings
}

func NewOnboardingService(repo OnboardingReader, sender SenderSettings) *OnboardingService {
	return &OnboardingService{repo: repo, sender: sender}
}

// Summary computes the checklist. complete requires all five conditions.
func (s *OnboardingService) Summary(ctx context.Context, scope coretenant.Scope) (OnboardingSummary, error) {
	c, err := s.repo.Counts(ctx, scope)
	if err != nil {
		return OnboardingSummary{}, err
	}
	hasSender, err := s.sender.HasDefaultSender(ctx, scope)
	if err != nil {
		return OnboardingSummary{}, err
	}
	out := OnboardingSummary{
		RouterRegistered:    c.Routers > 0,
		RouterReady:         c.RoutersReady > 0,
		SecretsTotal:        c.SecretsTotal,
		SecretsPaired:       c.SecretsPaired,
		SecretsNeedsProfile: c.NeedsProfile,
		ProductsMarked:      c.ProductsMarked,
		SenderConfigured:    hasSender,
	}
	out.Complete = out.RouterReady && out.SecretsPaired >= 1 && out.SecretsNeedsProfile == 0 &&
		out.ProductsMarked >= 1 && out.SenderConfigured
	return out, nil
}

// CustomerStatuses returns one entry per requested id, in request order.
// The caller has already validated and deduplicated the ids.
func (s *OnboardingService) CustomerStatuses(ctx context.Context, scope coretenant.Scope, accountIDs []string) ([]CustomerStatus, error) {
	found, err := s.repo.AccountStatuses(ctx, scope, accountIDs)
	if err != nil {
		return nil, err
	}
	out := make([]CustomerStatus, 0, len(accountIDs))
	for _, id := range accountIDs {
		st := found[id]
		state := CustomerNormal
		switch {
		case st.Secrets == 0:
			state = CustomerNone
		case st.Isolated:
			state = CustomerIsolated
		}
		out = append(out, CustomerStatus{AccountID: id, IsolationState: state, Exempt: st.Exempt, Secrets: st.Secrets, ExemptedUntil: st.ExemptedUntil})
	}
	return out, nil
}
