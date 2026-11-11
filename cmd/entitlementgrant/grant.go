package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

// maxDays keeps a platform override from becoming a forgotten permanent grant.
const maxDays = 3650

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// grantWindow returns the effective window in UTC. organization_entitlements
// stores "timestamp without time zone" and the application compares against
// UTC, so a local-time value (WIB, +7h) would make the grant start hours in
// the future and every endpoint answer 403 until then. from is one minute
// early to absorb clock skew between this host and the database.
func grantWindow(now time.Time, days int) (from, until time.Time) {
	utc := now.UTC()
	return utc.Add(-time.Minute), utc.AddDate(0, 0, days)
}

type grantInput struct {
	OrganizationID string
	FeatureKey     string
	Days           int
	Reason         string
	ActorUserID    string
}

// upserter is the subset of repository.EntitlementRepository the CLI uses.
type upserter interface {
	Upsert(ctx context.Context, p repository.UpsertEntitlementParams) (model.Entitlement, error)
}

func (in grantInput) validate() error {
	switch {
	case !uuidPattern.MatchString(in.OrganizationID):
		return errors.New("--org must be an organization uuid")
	case strings.TrimSpace(in.FeatureKey) == "":
		return errors.New("--feature is required")
	case in.Days < 1 || in.Days > maxDays:
		return fmt.Errorf("--days must be between 1 and %d", maxDays)
	case strings.TrimSpace(in.Reason) == "":
		return errors.New("--reason is required (platform_override is audited)")
	case !uuidPattern.MatchString(in.ActorUserID):
		return errors.New("--by must be the uuid of the user granting this")
	}
	return nil
}

// grant validates before touching the database. Upsert is keyed by
// (organization, feature, source, source_reference), so repeating the command
// updates the existing override instead of adding a second row.
func grant(ctx context.Context, repo upserter, in grantInput, now time.Time) (model.Entitlement, error) {
	if err := in.validate(); err != nil {
		return model.Entitlement{}, err
	}
	from, until := grantWindow(now, in.Days)
	return repo.Upsert(ctx, repository.UpsertEntitlementParams{
		OrganizationID: strings.TrimSpace(in.OrganizationID),
		FeatureKey:     strings.TrimSpace(in.FeatureKey),
		Source:         model.EntitlementSourcePlatformOverride,
		Status:         model.EntitlementStatusActive,
		EffectiveFrom:  from,
		EffectiveUntil: &until,
		Reason:         strings.TrimSpace(in.Reason),
		ActorUserID:    strings.TrimSpace(in.ActorUserID),
	})
}
