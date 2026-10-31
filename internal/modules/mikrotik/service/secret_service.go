package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// MaxImportSecrets bounds one import so a hostile or huge router cannot exhaust memory.
const MaxImportSecrets = 10000

const secretPath = "/ppp/secret"

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
	now     func() time.Time
}

func NewSecretService(routers *RouterService, repo repository.SecretRepository, audit Auditor) *SecretService {
	return &SecretService{routers: routers, repo: repo, audit: audit, now: time.Now}
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
