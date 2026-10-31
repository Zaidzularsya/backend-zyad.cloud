package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// MaxImportSecrets bounds one import so a hostile or huge router cannot exhaust memory.
const MaxImportSecrets = 10000

const (
	secretPath  = "/ppp/secret"
	profilePath = "/ppp/profile"
)

// Auditor writes audit rows. RouterRepository satisfies it.
type Auditor interface {
	InsertAudit(ctx context.Context, scope coretenant.Scope, entry repository.AuditEntry) error
}

// SecretService imports and maps PPP secrets. It reuses RouterService for
// router lookup and the client dial so host policy and credential handling stay
// in one place.
type SecretService struct {
	routers *RouterService
	repo    repository.SecretRepository
	audit   Auditor
	// accounts and provisioner are only needed by the pairing operations.
	accounts    AccountDirectory
	provisioner ContactProvisioner
	now         func() time.Time
}

func NewSecretService(routers *RouterService, repo repository.SecretRepository, audit Auditor, accounts AccountDirectory, provisioner ContactProvisioner) *SecretService {
	return &SecretService{routers: routers, repo: repo, audit: audit, accounts: accounts, provisioner: provisioner, now: time.Now}
}

// Import reads /ppp/secret from the router and syncs the local cache. Secrets
// that disappeared from the router are marked present=false, never deleted.
func (s *SecretService) Import(ctx context.Context, scope coretenant.Scope, routerID, actorUserID string) (repository.ImportResult, error) {
	router, client, secrets, err := s.routers.openClient(ctx, scope, routerID)
	if err != nil {
		return repository.ImportResult{}, err
	}
	rows, err := client.Print(ctx, secretPath, nil)
	if err != nil {
		return repository.ImportResult{}, translateClientError(err, secrets...)
	}
	if len(rows) > MaxImportSecrets {
		return repository.ImportResult{}, invalid("router has too many ppp secrets to import (limit 10000)")
	}

	imported := make([]repository.ImportedSecret, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row["name"])
		if name == "" || !isPPPoEService(row["service"]) {
			continue
		}
		profile := row["profile"]
		imported = append(imported, repository.ImportedSecret{
			Name:                 name,
			Profile:              profile,
			Comment:              row["comment"],
			Disabled:             isTrue(row["disabled"]),
			NeedsOriginalProfile: router.IsolirProfileName != "" && profile == router.IsolirProfileName,
		})
	}

	seenAt := s.now().UTC()
	res, err := s.repo.UpsertImported(ctx, scope, routerID, imported, seenAt)
	if err != nil {
		return repository.ImportResult{}, storeError(err, secrets...)
	}
	missing, err := s.repo.MarkMissing(ctx, scope, routerID, seenAt)
	if err != nil {
		return repository.ImportResult{}, storeError(err, secrets...)
	}
	res.Missing = missing

	// The import is already committed; a failed audit row must not turn it into an error.
	if err := s.audit.InsertAudit(ctx, scope, repository.AuditEntry{
		Event: "mikrotik.secrets_imported", ActorUserID: actorUserID, TargetID: routerID,
		Metadata: map[string]any{"target": "secret", "created": res.Created, "updated": res.Updated, "missing": res.Missing},
	}); err != nil {
		slog.Warn("mikrotik: audit insert failed", "router_id", routerID, "event", "mikrotik.secrets_imported", "error", scrub(err.Error(), secrets...))
	}
	return res, nil
}

// isPPPoEService keeps secrets usable by PPPoE: RouterOS treats a missing
// service as "any".
func isPPPoEService(service string) bool {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "", "any", "pppoe":
		return true
	}
	return false
}

func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes":
		return true
	}
	return false
}

// pairableSecret loads the secret and its router and checks the secret may be
// paired. The router's own isolir profile name is used, never an empty one.
func (s *SecretService) pairableSecret(ctx context.Context, scope coretenant.Scope, secretID string) (domain.PPPSecret, error) {
	secret, err := s.repo.FindByID(ctx, scope, secretID)
	if err != nil {
		return domain.PPPSecret{}, err
	}
	router, err := s.routers.Get(ctx, scope, secret.RouterID)
	if err != nil {
		return domain.PPPSecret{}, err
	}
	if secret.AccountID != nil {
		return domain.PPPSecret{}, mikrotik.ErrSecretAlreadyPaired
	}
	if err := secret.Pairable(router.IsolirProfileName); err != nil {
		return domain.PPPSecret{}, err
	}
	return secret, nil
}

// Pair links a secret to an existing account of the same organization.
func (s *SecretService) Pair(ctx context.Context, scope coretenant.Scope, secretID, accountID, actor string) (domain.PPPSecret, error) {
	secret, err := s.pairableSecret(ctx, scope, secretID)
	if err != nil {
		return domain.PPPSecret{}, err
	}
	account, err := s.accounts.Get(ctx, scope, accountID)
	if err != nil {
		return domain.PPPSecret{}, err
	}
	return s.commitPair(ctx, scope, secret, account.ID, actor)
}

// PairNewCustomer creates a CRM contact and account named after the secret and
// pairs them. Pairable is checked first so no contact is created for a secret
// that cannot be paired. If Pair then loses a race the new contact stays
// orphaned (accepted) and the conflict is returned.
func (s *SecretService) PairNewCustomer(ctx context.Context, scope coretenant.Scope, secretID, actor string) (domain.PPPSecret, error) {
	secret, err := s.pairableSecret(ctx, scope, secretID)
	if err != nil {
		return domain.PPPSecret{}, err
	}
	account, err := s.provisioner.CreateCustomerFromSecret(ctx, scope, secret.Name, secret.Comment, actor)
	if err != nil {
		return domain.PPPSecret{}, err
	}
	out, err := s.commitPair(ctx, scope, secret, account.ID, actor)
	if errors.Is(err, mikrotik.ErrSecretAlreadyPaired) {
		// No names or phones here: only identifiers.
		slog.Warn("mikrotik: secret was paired concurrently; created account left unpaired", "secret_id", secret.ID, "account_id", account.ID)
	}
	return out, err
}

func (s *SecretService) commitPair(ctx context.Context, scope coretenant.Scope, secret domain.PPPSecret, accountID, actor string) (domain.PPPSecret, error) {
	if err := s.repo.Pair(ctx, scope, secret.ID, accountID, actor); err != nil {
		return domain.PPPSecret{}, err
	}
	s.auditSecret(ctx, scope, "secret_paired", secret, accountID, actor)
	return s.repo.FindByID(ctx, scope, secret.ID)
}

// Unpair removes the customer link. Isolated secrets are refused so the
// original profile is restored first.
func (s *SecretService) Unpair(ctx context.Context, scope coretenant.Scope, secretID, actor string) error {
	secret, err := s.repo.FindByID(ctx, scope, secretID)
	if err != nil {
		return err
	}
	if err := s.repo.Unpair(ctx, scope, secretID); err != nil {
		return err
	}
	accountID := ""
	if secret.AccountID != nil {
		accountID = *secret.AccountID
	}
	s.auditSecret(ctx, scope, "secret_unpaired", secret, accountID, actor)
	return nil
}

// SetOriginalProfile records the profile to restore after isolir. The profile
// must exist on the router and must not be the isolir profile.
func (s *SecretService) SetOriginalProfile(ctx context.Context, scope coretenant.Scope, secretID, profile, actor string) error {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return invalid("original profile is required")
	}
	secret, err := s.repo.FindByID(ctx, scope, secretID)
	if err != nil {
		return err
	}
	router, client, secrets, err := s.routers.openClient(ctx, scope, secret.RouterID)
	if err != nil {
		return err
	}
	if router.IsolirProfileName != "" && profile == router.IsolirProfileName {
		return invalid("original profile cannot be the isolir profile")
	}
	rows, err := client.Print(ctx, profilePath, nil)
	if err != nil {
		return translateClientError(err, secrets...)
	}
	found := false
	for _, row := range rows {
		if row["name"] == profile {
			found = true
			break
		}
	}
	if !found {
		return invalid("profile does not exist on the router")
	}
	if err := s.repo.SetOriginalProfile(ctx, scope, secretID, profile); err != nil {
		return err
	}
	s.auditSecret(ctx, scope, "secret_original_profile_set", secret, "", actor)
	return nil
}

// auditSecret logs after the mutation committed; a failed audit row must not
// fail an operation that already succeeded.
func (s *SecretService) auditSecret(ctx context.Context, scope coretenant.Scope, event string, secret domain.PPPSecret, accountID, actor string) {
	meta := map[string]any{"target": "secret", "secret_id": secret.ID}
	if accountID != "" {
		meta["account_id"] = accountID
	}
	if err := s.audit.InsertAudit(ctx, scope, repository.AuditEntry{Event: event, ActorUserID: actor, TargetID: secret.ID, Metadata: meta}); err != nil {
		slog.Warn("mikrotik: audit insert failed", "secret_id", secret.ID, "event", event, "error", scrub(err.Error()))
	}
}
