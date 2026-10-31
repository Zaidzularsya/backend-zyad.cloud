package service

import (
	"context"
	"log/slog"
	"strings"
	"unicode/utf8"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

const maxExemptReasonRunes = 500

// PolicyService manages per-customer isolir policy (and, later, product triggers).
type PolicyService struct {
	repo     repository.PolicyRepository
	accounts AccountDirectory
	audit    Auditor
	// onChange runs after a successful policy write so a later phase can
	// re-evaluate the customer. nil = no-op.
	onChange func(ctx context.Context, scope coretenant.Scope, accountID string)
}

func NewPolicyService(repo repository.PolicyRepository, accounts AccountDirectory, audit Auditor, onChange func(ctx context.Context, scope coretenant.Scope, accountID string)) *PolicyService {
	return &PolicyService{repo: repo, accounts: accounts, audit: audit, onChange: onChange}
}

// SetExempt marks or unmarks a customer as exempt from isolir. The account is
// resolved through AccountDirectory first, so an account of another
// organization yields ErrAccountNotFound without touching the policy table.
func (s *PolicyService) SetExempt(ctx context.Context, scope coretenant.Scope, accountID string, exempt bool, reason, actor string) (domain.CustomerPolicy, error) {
	reason = strings.TrimSpace(reason)
	n := utf8.RuneCountInString(reason)
	if n > maxExemptReasonRunes || (exempt && n == 0) {
		return domain.CustomerPolicy{}, invalid("reason is required (1-500 characters) when exempting a customer")
	}
	if _, err := s.accounts.Get(ctx, scope, accountID); err != nil {
		return domain.CustomerPolicy{}, err
	}
	p, err := s.repo.UpsertCustomerPolicy(ctx, scope, accountID, exempt, reason, actor)
	if err != nil {
		return domain.CustomerPolicy{}, err
	}
	// The write is committed; a failed audit row must not fail it.
	if err := s.audit.InsertAudit(ctx, scope, repository.AuditEntry{
		Event: "customer_policy.exempt_set", ActorUserID: actor, TargetID: accountID,
		Metadata: map[string]any{"target": "customer_policy", "account_id": accountID, "exempt": exempt, "reason": reason},
	}); err != nil {
		slog.Warn("mikrotik: audit insert failed", "account_id", accountID, "event", "customer_policy.exempt_set", "error", scrub(err.Error()))
	}
	if s.onChange != nil {
		s.onChange(ctx, scope, accountID)
	}
	return p, nil
}

// GetPolicy returns the customer's policy, defaulting to not exempt.
func (s *PolicyService) GetPolicy(ctx context.Context, scope coretenant.Scope, accountID string) (domain.CustomerPolicy, error) {
	if _, err := s.accounts.Get(ctx, scope, accountID); err != nil {
		return domain.CustomerPolicy{}, err
	}
	p, found, err := s.repo.GetCustomerPolicy(ctx, scope, accountID)
	if err != nil {
		return domain.CustomerPolicy{}, err
	}
	if !found {
		return domain.CustomerPolicy{OrganizationID: scope.OrganizationID(), AccountID: accountID}, nil
	}
	return p, nil
}
