package app

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/service"
)

// publicLookupMax mirrors the repository cap: enough rows to tell 0, 1 and ambiguous.
const publicLookupMax = 5

// AccountsByPhone matches normalized phones in one organization-scoped query.
// normalize_phone_id (migration 000125) is the SQL twin of phone.NormalizeID.
func (r mikrotikReceivableReader) AccountsByPhone(ctx context.Context, scope coretenant.Scope, normalizedPhone string) ([]service.AccountRef, error) {
	out := []service.AccountRef{}
	err := r.scopedRead(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, name, COALESCE(phone, ''), COALESCE(email, '')
			FROM receivable_accounts
			WHERE organization_id = $1 AND deleted_at IS NULL
			  AND normalize_phone_id(phone) = $2
			ORDER BY id
			LIMIT $3`, scope.OrganizationID(), normalizedPhone, publicLookupMax)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a service.AccountRef
			if err := rows.Scan(&a.ID, &a.Name, &a.Phone, &a.Email); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

type mikrotikPublicNames interface {
	AccountIDsByName(ctx context.Context, scope coretenant.Scope, name string) ([]string, error)
}

type mikrotikPublicPhones interface {
	AccountsByPhone(ctx context.Context, scope coretenant.Scope, normalizedPhone string) ([]service.AccountRef, error)
}

type mikrotikPublicFeatures interface {
	RequireFeature(ctx context.Context, organizationID, featureKey string) error
}

// mikrotikPublicAdapter satisfies the service ports of the public isolir page
// (PublicDirectory, PublicPayLinks, PublicTenantProfile, PublicFeatureGate).
type mikrotikPublicAdapter struct {
	names    mikrotikPublicNames
	phones   mikrotikPublicPhones
	accounts mikrotikNotifAccounts
	settings mikrotikNotifSettings
	links    payLinker
	issuer   mikrotikNotifIssuer
	features mikrotikPublicFeatures
}

var (
	_ service.PublicDirectory     = mikrotikPublicAdapter{}
	_ service.PublicPayLinks      = mikrotikPublicAdapter{}
	_ service.PublicTenantProfile = mikrotikPublicAdapter{}
	_ service.PublicFeatureGate   = mikrotikPublicAdapter{}
)

func newMikrotikPublicAdapter(names mikrotikPublicNames, phones mikrotikPublicPhones, accounts mikrotikNotifAccounts,
	settings mikrotikNotifSettings, links payLinker, issuer mikrotikNotifIssuer, features mikrotikPublicFeatures) mikrotikPublicAdapter {
	return mikrotikPublicAdapter{names: names, phones: phones, accounts: accounts, settings: settings, links: links, issuer: issuer, features: features}
}

// AccountsByUsername: one DISTINCT scoped id query (<= 5 rows), then at most 5 account reads.
func (a mikrotikPublicAdapter) AccountsByUsername(ctx context.Context, scope coretenant.Scope, username string) ([]service.AccountRef, error) {
	ids, err := a.names.AccountIDsByName(ctx, scope, username)
	if err != nil {
		return nil, err
	}
	out := make([]service.AccountRef, 0, len(ids))
	for _, id := range ids {
		acc, err := a.accounts.Get(ctx, scope, id)
		if err != nil {
			return nil, err
		}
		out = append(out, accountRef(acc))
	}
	return out, nil
}

func (a mikrotikPublicAdapter) AccountsByPhone(ctx context.Context, scope coretenant.Scope, normalizedPhone string) ([]service.AccountRef, error) {
	return a.phones.AccountsByPhone(ctx, scope, normalizedPhone)
}

// PayLink returns "" when the tenant has no default sender (no link can be built).
func (a mikrotikPublicAdapter) PayLink(ctx context.Context, scope coretenant.Scope, invoiceID string) (string, error) {
	settings, err := a.settings.Get(ctx, scope)
	if err != nil {
		return "", err
	}
	sender := strings.TrimSpace(settings.DefaultSenderUserID)
	if sender == "" {
		return "", nil
	}
	link, _, err := a.links.Link(ctx, scope, invoiceID, sender)
	if err != nil {
		return "", err
	}
	return link, nil
}

func (a mikrotikPublicAdapter) Profile(ctx context.Context, scope coretenant.Scope) (service.TenantContact, error) {
	party, err := a.issuer.OrganizationProfile(ctx, scope)
	if err != nil {
		return service.TenantContact{}, err
	}
	return service.TenantContact{Name: party.Name, Phone: party.Phone, Email: party.Email}, nil
}

// Enabled reports the plain entitlement denial as (false, nil); other errors surface.
func (a mikrotikPublicAdapter) Enabled(ctx context.Context, scope coretenant.Scope) (bool, error) {
	err := a.features.RequireFeature(ctx, scope.OrganizationID(), mikrotikFeatureKey)
	if err == nil {
		return true, nil
	}
	if isFeatureNotEnabled(err) {
		return false, nil
	}
	return false, err
}
