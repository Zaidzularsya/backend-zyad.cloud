package seeder

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/platform/database"
)

const internalEntitlementFeatureKey = "platform.internal"

type PlatformOrganizationResult struct {
	OrganizationID string
	OwnerUserID    string
	MembershipID   string
	EntitlementID  string
	DomainIDs      []string
}

func SeedPlatformOrganization(
	ctx context.Context,
	db *database.Pool,
	multiTenant config.MultiTenantConfig,
	seedAdmin config.SeedAdminConfig,
) (PlatformOrganizationResult, error) {
	if db == nil {
		return PlatformOrganizationResult{}, errors.New("database pool is required")
	}
	platformID := strings.TrimSpace(multiTenant.PlatformOrganizationID)
	platformSlug := strings.ToLower(strings.TrimSpace(multiTenant.PlatformOrganizationSlug))
	platformName := strings.TrimSpace(multiTenant.PlatformOrganizationName)
	if platformSlug == "" || platformName == "" {
		return PlatformOrganizationResult{},
			errors.New("PLATFORM_ORGANIZATION_SLUG and PLATFORM_ORGANIZATION_NAME are required")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return PlatformOrganizationResult{}, err
	}
	defer tx.Rollback(ctx)

	organizationID, err := upsertPlatformOrganization(
		ctx,
		tx,
		platformID,
		platformSlug,
		platformName,
		multiTenant.PlatformPrimaryDomain,
	)
	if err != nil {
		return PlatformOrganizationResult{}, err
	}

	ownerUserID, roleID, err := findPlatformOwner(ctx, tx, seedAdmin)
	if err != nil {
		return PlatformOrganizationResult{}, err
	}

	membershipID, err := upsertPlatformOwnerMembership(
		ctx,
		tx,
		organizationID,
		ownerUserID,
	)
	if err != nil {
		return PlatformOrganizationResult{}, err
	}

	if err := assignPlatformOwnerRole(
		ctx,
		tx,
		organizationID,
		ownerUserID,
		roleID,
	); err != nil {
		return PlatformOrganizationResult{}, err
	}

	entitlementID, err := upsertInternalPlanEntitlement(
		ctx,
		tx,
		organizationID,
		ownerUserID,
	)
	if err != nil {
		return PlatformOrganizationResult{}, err
	}
	domainIDs, err := upsertPlatformDomains(
		ctx,
		tx,
		organizationID,
		multiTenant.PlatformPrimaryDomain,
	)
	if err != nil {
		return PlatformOrganizationResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return PlatformOrganizationResult{}, err
	}

	return PlatformOrganizationResult{
		OrganizationID: organizationID,
		OwnerUserID:    ownerUserID,
		MembershipID:   membershipID,
		EntitlementID:  entitlementID,
		DomainIDs:      domainIDs,
	}, nil
}

func upsertPlatformOrganization(
	ctx context.Context,
	tx pgx.Tx,
	configuredID string,
	slug string,
	name string,
	primaryDomain string,
) (string, error) {
	var existingID string
	var existingType string
	err := tx.QueryRow(ctx, `
		SELECT id::text, type
		FROM organizations
		WHERE type = 'platform'
			OR lower(slug) = lower($1)
		ORDER BY CASE WHEN type = 'platform' THEN 0 ELSE 1 END, created_at
		LIMIT 1
		FOR UPDATE
	`, slug).Scan(&existingID, &existingType)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find platform organization: %w", err)
	}
	if existingID != "" {
		if existingType != "platform" {
			return "", fmt.Errorf("platform organization slug %q is already used by a customer organization", slug)
		}
		if configuredID != "" && existingID != configuredID {
			return "", fmt.Errorf(
				"PLATFORM_ORGANIZATION_ID %s does not match existing platform organization %s",
				configuredID,
				existingID,
			)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE organizations
			SET
				type = 'platform',
				slug = $2,
				name = $3,
				status = 'active',
				timezone = COALESCE(NULLIF(timezone, ''), 'Asia/Jakarta'),
				locale = COALESCE(NULLIF(locale, ''), 'id-ID'),
				data_placement = 'shared',
				metadata = jsonb_strip_nulls(
					metadata || jsonb_build_object(
						'platform_primary_domain',
						NULLIF($4, '')
					)
				),
				updated_at = now()
			WHERE id = $1::uuid
		`, existingID, slug, name, strings.TrimSpace(primaryDomain)); err != nil {
			return "", fmt.Errorf("update platform organization: %w", err)
		}
		return existingID, nil
	}

	var insertedID string
	if configuredID != "" {
		err = tx.QueryRow(ctx, `
			INSERT INTO organizations (
				id,
				type,
				slug,
				name,
				status,
				data_placement,
				metadata,
				created_at,
				updated_at
			)
			VALUES (
				$1::uuid,
				'platform',
				$2,
				$3,
				'active',
				'shared',
				jsonb_strip_nulls(jsonb_build_object(
					'platform_primary_domain',
					NULLIF($4, '')
				)),
				now(),
				now()
			)
			RETURNING id::text
		`, configuredID, slug, name, strings.TrimSpace(primaryDomain)).Scan(&insertedID)
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO organizations (
				type,
				slug,
				name,
				status,
				data_placement,
				metadata,
				created_at,
				updated_at
			)
			VALUES (
				'platform',
				$1,
				$2,
				'active',
				'shared',
				jsonb_strip_nulls(jsonb_build_object(
					'platform_primary_domain',
					NULLIF($3, '')
				)),
				now(),
				now()
			)
			RETURNING id::text
		`, slug, name, strings.TrimSpace(primaryDomain)).Scan(&insertedID)
	}
	if err != nil {
		return "", fmt.Errorf("create platform organization: %w", err)
	}
	return insertedID, nil
}

func findPlatformOwner(
	ctx context.Context,
	tx pgx.Tx,
	cfg config.SeedAdminConfig,
) (string, string, error) {
	role := strings.TrimSpace(cfg.Role)
	if role == "" {
		role = "super_admin"
	}
	email := strings.TrimSpace(cfg.Email)
	if email != "" {
		var userID string
		var roleID string
		err := tx.QueryRow(ctx, `
			SELECT user_account.id::text, role.id::text
			FROM users user_account
			JOIN user_roles user_role
				ON user_role.user_id = user_account.id
				AND user_role.organization_id IS NULL
			JOIN roles role ON role.id = user_role.role_id
			WHERE lower(user_account.email) = lower($1)
				AND user_account.status = 'active'
				AND user_account.deleted_at IS NULL
				AND (role.role_name = $2 OR role.slug = $2)
			LIMIT 1
		`, email, role).Scan(&userID, &roleID)
		if err == nil {
			return userID, roleID, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("find seed admin owner: %w", err)
		}
	}

	var userID string
	var roleID string
	if err := tx.QueryRow(ctx, `
		SELECT user_account.id::text, role.id::text
		FROM users user_account
		JOIN user_roles user_role
			ON user_role.user_id = user_account.id
			AND user_role.organization_id IS NULL
		JOIN roles role ON role.id = user_role.role_id
		WHERE user_account.status = 'active'
			AND user_account.deleted_at IS NULL
			AND (role.role_name = $1 OR role.slug = $1)
		ORDER BY user_role.assigned_at ASC, user_account.created_at ASC
		LIMIT 1
	`, role).Scan(&userID, &roleID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("active %s user is required before seeding platform organization", role)
		}
		return "", "", fmt.Errorf("find platform owner: %w", err)
	}
	return userID, roleID, nil
}

func upsertPlatformOwnerMembership(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	userID string,
) (string, error) {
	var membershipID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO organization_memberships (
			organization_id,
			user_id,
			status,
			is_owner,
			accepted_at,
			created_at,
			updated_at
		)
		VALUES ($1::uuid, $2::uuid, 'active', true, now(), now(), now())
		ON CONFLICT (user_id, organization_id)
		DO UPDATE SET
			status = 'active',
			is_owner = true,
			accepted_at = COALESCE(organization_memberships.accepted_at, now()),
			removed_at = NULL,
			suspended_at = NULL,
			updated_at = now()
		RETURNING id::text
	`, organizationID, userID).Scan(&membershipID); err != nil {
		return "", fmt.Errorf("upsert platform owner membership: %w", err)
	}
	return membershipID, nil
}

func assignPlatformOwnerRole(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	userID string,
	roleID string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO user_roles (
			user_id,
			role_id,
			organization_id,
			assigned_by,
			assigned_at
		)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $1::uuid, now())
		ON CONFLICT (user_id, role_id, organization_id)
		WHERE organization_id IS NOT NULL
		DO UPDATE SET
			assigned_by = EXCLUDED.assigned_by,
			assigned_at = EXCLUDED.assigned_at
	`, userID, roleID, organizationID)
	if err != nil {
		return fmt.Errorf("assign platform owner role: %w", err)
	}
	return nil
}

func upsertInternalPlanEntitlement(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	actorUserID string,
) (string, error) {
	var entitlementID string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM organization_entitlements
		WHERE organization_id = $1::uuid
			AND feature_key = $2
			AND source = 'plan'
			AND source_reference = 'platform-internal'
		ORDER BY version DESC, updated_at DESC
		LIMIT 1
		FOR UPDATE
	`, organizationID, internalEntitlementFeatureKey).Scan(&entitlementID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("upsert internal platform entitlement: %w", err)
	}
	if entitlementID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE organization_entitlements
			SET
				status = 'active',
				limits = '{}'::jsonb,
				effective_until = NULL,
				reason = 'internal platform entitlement',
				updated_by = $2::uuid,
				updated_at = now()
			WHERE id = $1::uuid
		`, entitlementID, actorUserID); err != nil {
			return "", fmt.Errorf("update internal platform entitlement: %w", err)
		}
		return entitlementID, nil
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO organization_entitlements (
			organization_id,
			feature_key,
			source,
			source_reference,
			status,
			limits,
			effective_from,
			reason,
			created_by,
			updated_by,
			created_at,
			updated_at
		)
		VALUES (
			$1::uuid,
			$2,
			'plan',
			'platform-internal',
			'active',
			'{}'::jsonb,
			now(),
			'internal platform entitlement',
			$3::uuid,
			$3::uuid,
			now(),
			now()
		)
		RETURNING id::text
	`, organizationID, internalEntitlementFeatureKey, actorUserID).Scan(&entitlementID); err != nil {
		return "", fmt.Errorf("create internal platform entitlement: %w", err)
	}
	return entitlementID, nil
}

func upsertPlatformDomains(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	primaryDomain string,
) ([]string, error) {
	primaryDomain = canonicalDomain(primaryDomain)
	if primaryDomain == "" {
		return nil, nil
	}
	hosts := []string{primaryDomain}
	wwwAlias := "www." + primaryDomain
	if wwwAlias != primaryDomain {
		hosts = append(hosts, wwwAlias)
	}
	domainIDs := make([]string, 0, len(hosts))
	for index, host := range hosts {
		domainID, err := upsertPlatformDomain(
			ctx,
			tx,
			organizationID,
			host,
			index == 0,
		)
		if err != nil {
			return nil, err
		}
		domainIDs = append(domainIDs, domainID)
	}
	return domainIDs, nil
}

func upsertPlatformDomain(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	host string,
	isPrimary bool,
) (string, error) {
	var domainID string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM organization_domains
		WHERE lower(canonical_host) = lower($1)
		FOR UPDATE
	`, host).Scan(&domainID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find platform domain %s: %w", host, err)
	}
	if domainID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE organization_domains
			SET
				organization_id = $2::uuid,
				type = 'platform',
				status = 'active',
				is_primary = $3,
				verification_challenge_hash = NULL,
				verification_error = NULL,
				verified_at = COALESCE(verified_at, now()),
				ssl_status = 'active',
				ssl_error = NULL,
				deleted_at = NULL,
				updated_at = now()
			WHERE id = $1::uuid
		`, domainID, organizationID, isPrimary); err != nil {
			return "", fmt.Errorf("update platform domain %s: %w", host, err)
		}
		return domainID, nil
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO organization_domains (
			organization_id,
			type,
			canonical_host,
			status,
			is_primary,
			verified_at,
			ssl_status,
			created_at,
			updated_at
		)
		VALUES (
			$1::uuid,
			'platform',
			$2,
			'active',
			$3,
			now(),
			'active',
			now(),
			now()
		)
		RETURNING id::text
	`, organizationID, host, isPrimary).Scan(&domainID); err != nil {
		return "", fmt.Errorf("create platform domain %s: %w", host, err)
	}
	return domainID, nil
}

func canonicalDomain(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}
