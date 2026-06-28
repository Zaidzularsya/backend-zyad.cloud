package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

const domainSelectColumns = `
	id,
	organization_id,
	type,
	canonical_host,
	status,
	is_primary,
	COALESCE(verification_challenge_hash, ''),
	verification_attempts,
	last_verification_at,
	verified_at,
	COALESCE(verification_error, ''),
	ssl_status,
	COALESCE(ssl_error, ''),
	ssl_expires_at,
	created_at,
	updated_at,
	deleted_at
`

type DomainRepository struct {
	db *database.Pool
}

type CreateDomainParams struct {
	OrganizationID            string
	Type                      model.DomainType
	CanonicalHost             string
	VerificationChallengeHash string
	SSLStatus                 model.DomainSSLStatus
	ActorUserID               string
}

type DomainListFilter struct {
	Type           model.DomainType
	Status         model.DomainStatus
	IncludeDeleted bool
	Limit          int
	Offset         int
}

type UpdateDomainVerificationParams struct {
	Status            model.DomainStatus
	VerificationError string
	VerifiedAt        *time.Time
	AttemptedAt       time.Time
	ActorUserID       string
}

type UpdateDomainSSLParams struct {
	Status    model.DomainSSLStatus
	Error     string
	ExpiresAt *time.Time
	UpdatedAt time.Time
}

type ResolvedDomain struct {
	Domain       model.OrganizationDomain
	Organization model.Organization
}

func NewDomainRepository(db *database.Pool) *DomainRepository {
	return &DomainRepository{db: db}
}

func (r *DomainRepository) Create(
	ctx context.Context,
	params CreateDomainParams,
) (model.OrganizationDomain, error) {
	sslStatus := params.SSLStatus
	if sslStatus == "" {
		sslStatus = model.DomainSSLStatusPending
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	defer tx.Rollback(ctx)

	var domain model.OrganizationDomain
	err = tx.QueryRow(ctx, `
		INSERT INTO organization_domains (
			organization_id,
			type,
			canonical_host,
			status,
			is_primary,
			verification_challenge_hash,
			ssl_status
		)
		SELECT
			organization.id,
			$2,
			$3,
			'pending',
			false,
			NULLIF($4, ''),
			$5
		FROM organizations organization
		WHERE organization.id = $1::uuid
			AND organization.deleted_at IS NULL
			AND organization.status <> 'archived'
		RETURNING `+domainSelectColumns,
		strings.TrimSpace(params.OrganizationID),
		string(params.Type),
		canonicalHost(params.CanonicalHost),
		strings.TrimSpace(params.VerificationChallengeHash),
		string(sslStatus),
	).Scan(domainScanDest(&domain)...)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	if err := insertDomainAudit(
		ctx,
		tx,
		domain,
		params.ActorUserID,
		"organization_domain_created",
		map[string]any{
			"domain_id":      domain.ID,
			"canonical_host": domain.CanonicalHost,
			"type":           string(domain.Type),
			"status":         string(domain.Status),
			"ssl_status":     string(domain.SSLStatus),
		},
		domain.CreatedAt,
	); err != nil {
		return model.OrganizationDomain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.OrganizationDomain{}, err
	}
	return domain, nil
}

func (r *DomainRepository) FindByID(
	ctx context.Context,
	organizationID string,
	id string,
) (model.OrganizationDomain, error) {
	var domain model.OrganizationDomain
	err := r.db.QueryRow(ctx, `
		SELECT `+domainSelectColumns+`
		FROM organization_domains
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
			AND deleted_at IS NULL
	`, strings.TrimSpace(id), strings.TrimSpace(organizationID)).Scan(domainScanDest(&domain)...)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	return domain, nil
}

func (r *DomainRepository) ListByOrganization(
	ctx context.Context,
	organizationID string,
	filter DomainListFilter,
) ([]model.OrganizationDomain, int64, error) {
	where, args := domainWhere(filter)
	args = append([]any{strings.TrimSpace(organizationID)}, args...)
	where = " WHERE organization_id = $1::uuid" + renumberDomainWhere(where, 1)

	var total int64
	if err := r.db.QueryRow(
		ctx,
		"SELECT count(*) FROM organization_domains"+where,
		args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := domainPagination(filter)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+domainSelectColumns+`
		FROM organization_domains`+where+`
		ORDER BY is_primary DESC, created_at ASC, id ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	domains := make([]model.OrganizationDomain, 0)
	for rows.Next() {
		var domain model.OrganizationDomain
		if err := rows.Scan(domainScanDest(&domain)...); err != nil {
			return nil, 0, err
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return domains, total, nil
}

func (r *DomainRepository) UpdateChallenge(
	ctx context.Context,
	organizationID string,
	id string,
	challengeHash string,
	updatedAt time.Time,
) (model.OrganizationDomain, error) {
	var domain model.OrganizationDomain
	err := r.db.QueryRow(ctx, `
		UPDATE organization_domains
		SET
			status = 'pending',
			is_primary = false,
			verification_challenge_hash = $3,
			verification_attempts = 0,
			last_verification_at = NULL,
			verified_at = NULL,
			verification_error = NULL,
			ssl_status = 'pending',
			ssl_error = NULL,
			ssl_expires_at = NULL,
			updated_at = $4
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
			AND deleted_at IS NULL
		RETURNING `+domainSelectColumns,
		strings.TrimSpace(id),
		strings.TrimSpace(organizationID),
		strings.TrimSpace(challengeHash),
		updatedAt,
	).Scan(domainScanDest(&domain)...)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	return domain, nil
}

func (r *DomainRepository) UpdateVerification(
	ctx context.Context,
	organizationID string,
	id string,
	params UpdateDomainVerificationParams,
) (model.OrganizationDomain, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	defer tx.Rollback(ctx)

	var domain model.OrganizationDomain
	err = tx.QueryRow(ctx, `
		UPDATE organization_domains
		SET
			status = $3,
			verification_attempts = verification_attempts + 1,
			last_verification_at = $4,
			verified_at = CASE
				WHEN $3::varchar IN ('verified', 'active') THEN $5
				ELSE verified_at
			END,
			verification_error = NULLIF($6, ''),
			updated_at = $4
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
			AND deleted_at IS NULL
		RETURNING `+domainSelectColumns,
		strings.TrimSpace(id),
		strings.TrimSpace(organizationID),
		string(params.Status),
		params.AttemptedAt,
		params.VerifiedAt,
		strings.TrimSpace(params.VerificationError),
	).Scan(domainScanDest(&domain)...)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	if err := insertDomainAudit(
		ctx,
		tx,
		domain,
		params.ActorUserID,
		"organization_domain_verification_recorded",
		map[string]any{
			"domain_id":             domain.ID,
			"canonical_host":        domain.CanonicalHost,
			"type":                  string(domain.Type),
			"status":                string(domain.Status),
			"verification_attempts": domain.VerificationAttempts,
			"verification_error":    domain.VerificationError,
			"verified":              domain.VerifiedAt != nil,
		},
		params.AttemptedAt,
	); err != nil {
		return model.OrganizationDomain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.OrganizationDomain{}, err
	}
	return domain, nil
}

func (r *DomainRepository) Activate(
	ctx context.Context,
	organizationID string,
	id string,
	isPrimary bool,
	activatedAt time.Time,
	actorUserID string,
) (model.OrganizationDomain, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	defer tx.Rollback(ctx)

	if err := lockOrganization(ctx, tx, organizationID); err != nil {
		return model.OrganizationDomain{}, err
	}
	domain, err := lockVerifiedDomain(ctx, tx, organizationID, id)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	if !isPrimary {
		var hasPrimary bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM organization_domains
				WHERE organization_id = $1::uuid
					AND is_primary = true
					AND status = 'active'
					AND deleted_at IS NULL
			)
		`, domain.OrganizationID).Scan(&hasPrimary); err != nil {
			return model.OrganizationDomain{}, err
		}
		isPrimary = !hasPrimary
	}
	if isPrimary {
		if _, err := tx.Exec(ctx, `
			UPDATE organization_domains
			SET is_primary = false, updated_at = $2
			WHERE organization_id = $1::uuid
				AND id <> $3::uuid
				AND is_primary = true
				AND deleted_at IS NULL
		`, domain.OrganizationID, activatedAt, domain.ID); err != nil {
			return model.OrganizationDomain{}, err
		}
	}

	err = tx.QueryRow(ctx, `
		UPDATE organization_domains
		SET status = 'active', is_primary = $3, updated_at = $4
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
			AND verified_at IS NOT NULL
			AND deleted_at IS NULL
		RETURNING `+domainSelectColumns,
		domain.ID,
		domain.OrganizationID,
		isPrimary,
		activatedAt,
	).Scan(domainScanDest(&domain)...)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	event := "organization_domain_activated"
	if isPrimary {
		event = "organization_domain_primary_set"
	}
	if err := insertDomainAudit(
		ctx,
		tx,
		domain,
		actorUserID,
		event,
		map[string]any{
			"domain_id":      domain.ID,
			"canonical_host": domain.CanonicalHost,
			"type":           string(domain.Type),
			"status":         string(domain.Status),
			"is_primary":     domain.IsPrimary,
		},
		activatedAt,
	); err != nil {
		return model.OrganizationDomain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.OrganizationDomain{}, err
	}
	return domain, nil
}

func (r *DomainRepository) SetPrimary(
	ctx context.Context,
	organizationID string,
	id string,
	updatedAt time.Time,
	actorUserID string,
) (model.OrganizationDomain, error) {
	return r.Activate(ctx, organizationID, id, true, updatedAt, actorUserID)
}

func (r *DomainRepository) UpdateSSL(
	ctx context.Context,
	organizationID string,
	id string,
	params UpdateDomainSSLParams,
) (model.OrganizationDomain, error) {
	var domain model.OrganizationDomain
	err := r.db.QueryRow(ctx, `
		UPDATE organization_domains
		SET
			ssl_status = $3,
			ssl_error = NULLIF($4, ''),
			ssl_expires_at = $5,
			updated_at = $6
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
			AND deleted_at IS NULL
		RETURNING `+domainSelectColumns,
		strings.TrimSpace(id),
		strings.TrimSpace(organizationID),
		string(params.Status),
		strings.TrimSpace(params.Error),
		params.ExpiresAt,
		params.UpdatedAt,
	).Scan(domainScanDest(&domain)...)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	return domain, nil
}

func (r *DomainRepository) Reassign(
	ctx context.Context,
	id string,
	fromOrganizationID string,
	toOrganizationID string,
	domainType model.DomainType,
	host string,
	challengeHash string,
	updatedAt time.Time,
) (model.OrganizationDomain, error) {
	var domain model.OrganizationDomain
	err := r.db.QueryRow(ctx, `
		UPDATE organization_domains AS domain
		SET
			organization_id = target.id,
			type = $4,
			canonical_host = $5,
			status = 'pending',
			is_primary = false,
			verification_challenge_hash = $6,
			verification_attempts = 0,
			last_verification_at = NULL,
			verified_at = NULL,
			verification_error = NULL,
			ssl_status = 'pending',
			ssl_error = NULL,
			ssl_expires_at = NULL,
			updated_at = $7
		FROM organizations target
		WHERE domain.id = $1::uuid
			AND domain.organization_id = $2::uuid
			AND target.id = $3::uuid
			AND target.deleted_at IS NULL
			AND target.status <> 'archived'
			AND domain.deleted_at IS NULL
		RETURNING `+prefixedDomainSelectColumns("domain"),
		strings.TrimSpace(id),
		strings.TrimSpace(fromOrganizationID),
		strings.TrimSpace(toOrganizationID),
		string(domainType),
		canonicalHost(host),
		strings.TrimSpace(challengeHash),
		updatedAt,
	).Scan(domainScanDest(&domain)...)
	if err != nil {
		return model.OrganizationDomain{}, err
	}
	return domain, nil
}

func (r *DomainRepository) Delete(
	ctx context.Context,
	organizationID string,
	id string,
	deletedAt time.Time,
	actorUserID string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var domain model.OrganizationDomain
	err = tx.QueryRow(ctx, `
		UPDATE organization_domains
		SET
			status = 'disabled',
			is_primary = false,
			deleted_at = $3,
			updated_at = $3
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
			AND deleted_at IS NULL
		RETURNING `+domainSelectColumns,
		strings.TrimSpace(id), strings.TrimSpace(organizationID), deletedAt).
		Scan(domainScanDest(&domain)...)
	if err != nil {
		return err
	}
	if err := insertDomainAudit(
		ctx,
		tx,
		domain,
		actorUserID,
		"organization_domain_disabled",
		map[string]any{
			"domain_id":      domain.ID,
			"canonical_host": domain.CanonicalHost,
			"type":           string(domain.Type),
			"status":         string(domain.Status),
		},
		deletedAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *DomainRepository) ResolveActiveHost(
	ctx context.Context,
	host string,
) (ResolvedDomain, error) {
	var result ResolvedDomain
	var metadataBytes []byte
	dest := domainScanDest(&result.Domain)
	dest = append(dest, organizationScanDest(&result.Organization, &metadataBytes)...)

	err := r.db.QueryRow(ctx, `
		SELECT
			`+prefixedDomainSelectColumns("domain")+`,
			`+prefixedOrganizationSelectColumns("organization")+`
		FROM organization_domains domain
		JOIN organizations organization ON organization.id = domain.organization_id
		WHERE domain.canonical_host = $1
			AND domain.status = 'active'
			AND domain.verified_at IS NOT NULL
			AND domain.deleted_at IS NULL
			AND organization.status = 'active'
			AND organization.deleted_at IS NULL
		LIMIT 1
	`, canonicalHost(host)).Scan(dest...)
	if err != nil {
		return ResolvedDomain{}, err
	}
	if err := decodeMetadata(metadataBytes, &result.Organization.Metadata); err != nil {
		return ResolvedDomain{}, err
	}
	return result, nil
}

func insertDomainAudit(
	ctx context.Context,
	tx pgx.Tx,
	domain model.OrganizationDomain,
	actorUserID string,
	event string,
	metadata map[string]any,
	createdAt time.Time,
) error {
	return insertOrganizationAuditTx(
		ctx,
		tx,
		model.Organization{ID: domain.OrganizationID},
		model.Membership{},
		strings.TrimSpace(actorUserID),
		event,
		metadata,
		createdAt,
	)
}

func domainWhere(filter DomainListFilter) (string, []any) {
	var query strings.Builder
	query.WriteString(" WHERE 1 = 1")
	args := make([]any, 0, 2)
	if !filter.IncludeDeleted {
		query.WriteString(" AND deleted_at IS NULL")
	}
	if filter.Type != "" {
		args = append(args, string(filter.Type))
		query.WriteString(fmt.Sprintf(" AND type = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		query.WriteString(fmt.Sprintf(" AND status = $%d", len(args)))
	}
	return query.String(), args
}

func renumberDomainWhere(where string, offset int) string {
	for index := 10; index >= 1; index-- {
		where = strings.ReplaceAll(
			where,
			fmt.Sprintf("$%d", index),
			fmt.Sprintf("$%d", index+offset),
		)
	}
	return strings.TrimPrefix(where, " WHERE 1 = 1")
}

func domainPagination(filter DomainListFilter) (int, int) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func domainScanDest(domain *model.OrganizationDomain) []any {
	return []any{
		&domain.ID,
		&domain.OrganizationID,
		&domain.Type,
		&domain.CanonicalHost,
		&domain.Status,
		&domain.IsPrimary,
		&domain.VerificationChallengeHash,
		&domain.VerificationAttempts,
		&domain.LastVerificationAt,
		&domain.VerifiedAt,
		&domain.VerificationError,
		&domain.SSLStatus,
		&domain.SSLError,
		&domain.SSLExpiresAt,
		&domain.CreatedAt,
		&domain.UpdatedAt,
		&domain.DeletedAt,
	}
}

func lockVerifiedDomain(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	id string,
) (model.OrganizationDomain, error) {
	var domain model.OrganizationDomain
	err := tx.QueryRow(ctx, `
		SELECT `+domainSelectColumns+`
		FROM organization_domains
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
			AND verified_at IS NOT NULL
			AND status IN ('verified', 'active')
			AND deleted_at IS NULL
		FOR UPDATE
	`, strings.TrimSpace(id), strings.TrimSpace(organizationID)).Scan(domainScanDest(&domain)...)
	return domain, err
}

func prefixedDomainSelectColumns(alias string) string {
	return prefixSelectColumns(alias, domainSelectColumns)
}

func canonicalHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
