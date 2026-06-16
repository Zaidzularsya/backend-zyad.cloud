package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

const organizationSelectColumns = `
	id,
	type,
	slug,
	name,
	status,
	timezone,
	locale,
	COALESCE(region, ''),
	data_placement,
	metadata,
	created_at,
	updated_at,
	deleted_at
`

type OrganizationRepository struct {
	db *database.Pool
}

type CreateOrganizationParams struct {
	Type          coretenant.OrganizationType
	Slug          string
	Name          string
	Status        coretenant.OrganizationStatus
	Timezone      string
	Locale        string
	Region        string
	DataPlacement coretenant.DataPlacement
	Metadata      map[string]any
}

type UpdateOrganizationParams struct {
	Slug          *string
	Name          *string
	Timezone      *string
	Locale        *string
	Region        *string
	DataPlacement *coretenant.DataPlacement
	Metadata      *map[string]any
}

type OrganizationListFilter struct {
	Type            coretenant.OrganizationType
	Status          coretenant.OrganizationStatus
	DataPlacement   coretenant.DataPlacement
	Search          string
	IncludeArchived bool
	Limit           int
	Offset          int
}

type OrganizationControlPlaneSummary struct {
	ActiveOwnerCount       int64
	DomainCount            int64
	ActiveDomainCount      int64
	PendingDomainCount     int64
	FailedDomainCount      int64
	SSLFailedDomainCount   int64
	PrimaryActiveCount     int64
	EntitlementCount       int64
	ActiveEntitlementCount int64
	MaxEntitlementVersion  int64
}

func NewOrganizationRepository(db *database.Pool) *OrganizationRepository {
	return &OrganizationRepository{db: db}
}

func (r *OrganizationRepository) Create(ctx context.Context, params CreateOrganizationParams) (model.Organization, error) {
	metadata, err := encodeMetadata(params.Metadata)
	if err != nil {
		return model.Organization{}, err
	}

	status := params.Status
	if status == "" {
		status = coretenant.OrganizationStatusPending
	}
	timezone := strings.TrimSpace(params.Timezone)
	if timezone == "" {
		timezone = "Asia/Jakarta"
	}
	locale := strings.TrimSpace(params.Locale)
	if locale == "" {
		locale = "id-ID"
	}
	dataPlacement := params.DataPlacement
	if dataPlacement == "" {
		dataPlacement = coretenant.DataPlacementShared
	}

	var organization model.Organization
	var metadataBytes []byte
	err = r.db.QueryRow(ctx, `
		INSERT INTO organizations (
			type,
			slug,
			name,
			status,
			timezone,
			locale,
			region,
			data_placement,
			metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9::jsonb)
		RETURNING `+organizationSelectColumns,
		string(params.Type),
		canonicalSlug(params.Slug),
		strings.TrimSpace(params.Name),
		string(status),
		timezone,
		locale,
		strings.TrimSpace(params.Region),
		string(dataPlacement),
		string(metadata),
	).Scan(organizationScanDest(&organization, &metadataBytes)...)
	if err != nil {
		return model.Organization{}, err
	}
	if err := decodeMetadata(metadataBytes, &organization.Metadata); err != nil {
		return model.Organization{}, err
	}
	return organization, nil
}

func (r *OrganizationRepository) List(ctx context.Context, filter OrganizationListFilter) ([]model.Organization, int64, error) {
	where, args := organizationWhere(filter)

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM organizations"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

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
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, `
		SELECT `+organizationSelectColumns+`
		FROM organizations`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	organizations := make([]model.Organization, 0)
	for rows.Next() {
		organization, err := scanOrganization(rows)
		if err != nil {
			return nil, 0, err
		}
		organizations = append(organizations, organization)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return organizations, total, nil
}

func (r *OrganizationRepository) FindByID(ctx context.Context, id string) (model.Organization, error) {
	return r.findOne(ctx, "id = $1::uuid", strings.TrimSpace(id), false)
}

func (r *OrganizationRepository) FindByIDIncludingArchived(
	ctx context.Context,
	id string,
) (model.Organization, error) {
	return r.findOne(ctx, "id = $1::uuid", strings.TrimSpace(id), true)
}

func (r *OrganizationRepository) ControlPlaneSummary(
	ctx context.Context,
	organizationID string,
) (OrganizationControlPlaneSummary, error) {
	var summary OrganizationControlPlaneSummary
	err := r.db.QueryRow(ctx, `
		SELECT
			(
				SELECT count(*)
				FROM organization_memberships membership
				WHERE membership.organization_id = organization.id
					AND membership.is_owner = true
					AND membership.status = 'active'
					AND membership.removed_at IS NULL
			),
			(
				SELECT count(*)
				FROM organization_domains domain
				WHERE domain.organization_id = organization.id
					AND domain.deleted_at IS NULL
			),
			(
				SELECT count(*)
				FROM organization_domains domain
				WHERE domain.organization_id = organization.id
					AND domain.status = 'active'
					AND domain.deleted_at IS NULL
			),
			(
				SELECT count(*)
				FROM organization_domains domain
				WHERE domain.organization_id = organization.id
					AND domain.status IN ('pending', 'verified')
					AND domain.deleted_at IS NULL
			),
			(
				SELECT count(*)
				FROM organization_domains domain
				WHERE domain.organization_id = organization.id
					AND domain.status = 'failed'
					AND domain.deleted_at IS NULL
			),
			(
				SELECT count(*)
				FROM organization_domains domain
				WHERE domain.organization_id = organization.id
					AND domain.ssl_status = 'failed'
					AND domain.deleted_at IS NULL
			),
			(
				SELECT count(*)
				FROM organization_domains domain
				WHERE domain.organization_id = organization.id
					AND domain.status = 'active'
					AND domain.is_primary = true
					AND domain.deleted_at IS NULL
			),
			(
				SELECT count(*)
				FROM organization_entitlements entitlement
				WHERE entitlement.organization_id = organization.id
			),
			(
				SELECT count(*)
				FROM organization_entitlements entitlement
				WHERE entitlement.organization_id = organization.id
					AND entitlement.status = 'active'
			),
			(
				SELECT COALESCE(max(entitlement.version), 0)
				FROM organization_entitlements entitlement
				WHERE entitlement.organization_id = organization.id
			)
		FROM organizations organization
		WHERE organization.id = $1::uuid
	`, strings.TrimSpace(organizationID)).Scan(
		&summary.ActiveOwnerCount,
		&summary.DomainCount,
		&summary.ActiveDomainCount,
		&summary.PendingDomainCount,
		&summary.FailedDomainCount,
		&summary.SSLFailedDomainCount,
		&summary.PrimaryActiveCount,
		&summary.EntitlementCount,
		&summary.ActiveEntitlementCount,
		&summary.MaxEntitlementVersion,
	)
	return summary, err
}

func (r *OrganizationRepository) FindBySlug(ctx context.Context, slug string) (model.Organization, error) {
	return r.findOne(ctx, "slug = $1", canonicalSlug(slug), false)
}

func (r *OrganizationRepository) FindPlatform(ctx context.Context) (model.Organization, error) {
	return r.findOne(ctx, "type = 'platform'", nil, false)
}

func (r *OrganizationRepository) Update(
	ctx context.Context,
	id string,
	params UpdateOrganizationParams,
) (model.Organization, error) {
	setClauses := make([]string, 0, 8)
	args := make([]any, 0, 9)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if params.Slug != nil {
		setClauses = append(setClauses, "slug = "+addArg(canonicalSlug(*params.Slug)))
	}
	if params.Name != nil {
		setClauses = append(setClauses, "name = "+addArg(strings.TrimSpace(*params.Name)))
	}
	if params.Timezone != nil {
		setClauses = append(setClauses, "timezone = "+addArg(strings.TrimSpace(*params.Timezone)))
	}
	if params.Locale != nil {
		setClauses = append(setClauses, "locale = "+addArg(strings.TrimSpace(*params.Locale)))
	}
	if params.Region != nil {
		setClauses = append(setClauses, "region = NULLIF("+addArg(strings.TrimSpace(*params.Region))+", '')")
	}
	if params.DataPlacement != nil {
		setClauses = append(setClauses, "data_placement = "+addArg(string(*params.DataPlacement)))
	}
	if params.Metadata != nil {
		metadata, err := encodeMetadata(*params.Metadata)
		if err != nil {
			return model.Organization{}, err
		}
		setClauses = append(setClauses, "metadata = "+addArg(string(metadata))+"::jsonb")
	}
	if len(setClauses) == 0 {
		return r.FindByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, strings.TrimSpace(id))

	var organization model.Organization
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		UPDATE organizations
		SET `+strings.Join(setClauses, ", ")+`
		WHERE id = $`+fmt.Sprint(len(args))+`::uuid
			AND deleted_at IS NULL
			AND status <> 'archived'
		RETURNING `+organizationSelectColumns,
		args...,
	).Scan(organizationScanDest(&organization, &metadataBytes)...)
	if err != nil {
		return model.Organization{}, err
	}
	if err := decodeMetadata(metadataBytes, &organization.Metadata); err != nil {
		return model.Organization{}, err
	}
	return organization, nil
}

func (r *OrganizationRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status coretenant.OrganizationStatus,
) (model.Organization, error) {
	var organization model.Organization
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		UPDATE organizations
		SET status = $2, updated_at = now()
		WHERE id = $1::uuid
			AND deleted_at IS NULL
			AND status <> 'archived'
		RETURNING `+organizationSelectColumns,
		strings.TrimSpace(id),
		string(status),
	).Scan(organizationScanDest(&organization, &metadataBytes)...)
	if err != nil {
		return model.Organization{}, err
	}
	if err := decodeMetadata(metadataBytes, &organization.Metadata); err != nil {
		return model.Organization{}, err
	}
	return organization, nil
}

func (r *OrganizationRepository) Archive(ctx context.Context, id string, archivedAt time.Time) error {
	commandTag, err := r.db.Exec(ctx, `
		UPDATE organizations
		SET status = 'archived', deleted_at = $2, updated_at = $2
		WHERE id = $1::uuid
			AND deleted_at IS NULL
			AND status <> 'archived'
	`, strings.TrimSpace(id), archivedAt)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *OrganizationRepository) findOne(
	ctx context.Context,
	predicate string,
	value any,
	includeArchived bool,
) (model.Organization, error) {
	args := make([]any, 0, 1)
	if value != nil {
		args = append(args, value)
	}
	where := " WHERE " + predicate
	if !includeArchived {
		where += " AND deleted_at IS NULL AND status <> 'archived'"
	}

	var organization model.Organization
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+organizationSelectColumns+`
		FROM organizations`+where+`
		LIMIT 1
	`, args...).Scan(organizationScanDest(&organization, &metadataBytes)...)
	if err != nil {
		return model.Organization{}, err
	}
	if err := decodeMetadata(metadataBytes, &organization.Metadata); err != nil {
		return model.Organization{}, err
	}
	return organization, nil
}

func organizationWhere(filter OrganizationListFilter) (string, []any) {
	var query strings.Builder
	query.WriteString(" WHERE 1 = 1")
	args := make([]any, 0, 5)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if !filter.IncludeArchived {
		query.WriteString(" AND deleted_at IS NULL AND status <> 'archived'")
	}
	if filter.Type != "" {
		query.WriteString(" AND type = " + addArg(string(filter.Type)))
	}
	if filter.Status != "" {
		query.WriteString(" AND status = " + addArg(string(filter.Status)))
	}
	if filter.DataPlacement != "" {
		query.WriteString(" AND data_placement = " + addArg(string(filter.DataPlacement)))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		arg := addArg("%" + search + "%")
		query.WriteString(" AND (name ILIKE " + arg + " OR slug ILIKE " + arg + ")")
	}
	return query.String(), args
}

func organizationScanDest(organization *model.Organization, metadataBytes *[]byte) []any {
	return []any{
		&organization.ID,
		&organization.Type,
		&organization.Slug,
		&organization.Name,
		&organization.Status,
		&organization.Timezone,
		&organization.Locale,
		&organization.Region,
		&organization.DataPlacement,
		metadataBytes,
		&organization.CreatedAt,
		&organization.UpdatedAt,
		&organization.DeletedAt,
	}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOrganization(row rowScanner) (model.Organization, error) {
	var organization model.Organization
	var metadataBytes []byte
	if err := row.Scan(organizationScanDest(&organization, &metadataBytes)...); err != nil {
		return model.Organization{}, err
	}
	if err := decodeMetadata(metadataBytes, &organization.Metadata); err != nil {
		return model.Organization{}, err
	}
	return organization, nil
}

func encodeMetadata(metadata map[string]any) ([]byte, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return json.Marshal(metadata)
}

func decodeMetadata(data []byte, metadata *map[string]any) error {
	if len(data) == 0 {
		*metadata = map[string]any{}
		return nil
	}
	return json.Unmarshal(data, metadata)
}

func canonicalSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}
