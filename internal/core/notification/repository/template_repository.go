package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/database"
)

const templateSelectColumns = `
	id,
	code,
	name,
	COALESCE(description, ''),
	channel,
	locale,
	COALESCE(subject_template, ''),
	body_template,
	available_variables,
	sample_payload,
	status,
	is_system,
	is_active,
	version,
	COALESCE(created_by::text, ''),
	COALESCE(updated_by::text, ''),
	created_at,
	updated_at,
	deleted_at
`

type TemplateRepository struct {
	db *database.Pool
}

type TemplateListFilter struct {
	Code           string
	Channel        domain.Channel
	Locale         string
	Status         domain.TemplateStatus
	IsActive       *bool
	IncludeDeleted bool
	Limit          int
	Offset         int
}

func NewTemplateRepository(db *database.Pool) *TemplateRepository {
	return &TemplateRepository{db: db}
}

func (r *TemplateRepository) Create(ctx context.Context, template *domain.NotificationTemplate) error {
	availableVariables, err := encodeVariables(template.AvailableVariables)
	if err != nil {
		return err
	}
	samplePayload, err := encodeMap(template.SamplePayload)
	if err != nil {
		return err
	}

	if template.Locale == "" {
		template.Locale = "id-ID"
	}
	if template.Status == "" {
		template.Status = domain.TemplateStatusDraft
	}
	if template.Version == 0 {
		template.Version = 1
	}

	err = r.db.QueryRow(ctx, `
		INSERT INTO notification_templates (
			code,
			name,
			description,
			channel,
			locale,
			subject_template,
			body_template,
			available_variables,
			sample_payload,
			status,
			is_system,
			is_active,
			version,
			created_by,
			updated_by,
			created_at,
			updated_at
		)
		VALUES (
			$1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, ''), $7, $8::jsonb, $9::jsonb,
			$10, $11, $12, $13, NULLIF($14, '')::uuid, NULLIF($15, '')::uuid, now(), now()
		)
		RETURNING `+templateSelectColumns,
		template.Code,
		template.Name,
		template.Description,
		string(template.Channel),
		template.Locale,
		template.SubjectTemplate,
		template.BodyTemplate,
		string(availableVariables),
		string(samplePayload),
		string(template.Status),
		template.IsSystem,
		template.IsActive,
		template.Version,
		template.CreatedBy,
		template.UpdatedBy,
	).Scan(templateScanDest(template)...)
	if err != nil {
		return err
	}

	return nil
}

func (r *TemplateRepository) FindByID(ctx context.Context, id string) (domain.NotificationTemplate, error) {
	var template domain.NotificationTemplate
	err := r.db.QueryRow(ctx, `
		SELECT `+templateSelectColumns+`
		FROM notification_templates
		WHERE id = $1
	`, id).Scan(templateScanDest(&template)...)
	if err != nil {
		return domain.NotificationTemplate{}, err
	}
	return template, nil
}

func (r *TemplateRepository) FindActiveByCodeChannelLocale(ctx context.Context, code string, channel domain.Channel, locale string) (domain.NotificationTemplate, error) {
	if locale == "" {
		locale = "id-ID"
	}

	var template domain.NotificationTemplate
	err := r.db.QueryRow(ctx, `
		SELECT `+templateSelectColumns+`
		FROM notification_templates
		WHERE code = $1
			AND channel = $2
			AND locale = $3
			AND status = 'active'
			AND is_active = true
			AND deleted_at IS NULL
		ORDER BY version DESC
		LIMIT 1
	`, code, string(channel), locale).Scan(templateScanDest(&template)...)
	if err != nil {
		return domain.NotificationTemplate{}, err
	}
	return template, nil
}

func (r *TemplateRepository) List(ctx context.Context, filter TemplateListFilter) ([]domain.NotificationTemplate, error) {
	query := strings.Builder{}
	query.WriteString("SELECT ")
	query.WriteString(templateSelectColumns)
	query.WriteString(" FROM notification_templates WHERE 1 = 1")

	args := make([]any, 0)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if !filter.IncludeDeleted {
		query.WriteString(" AND deleted_at IS NULL")
	}
	if filter.Code != "" {
		query.WriteString(" AND code = ")
		query.WriteString(addArg(filter.Code))
	}
	if filter.Channel != "" {
		query.WriteString(" AND channel = ")
		query.WriteString(addArg(string(filter.Channel)))
	}
	if filter.Locale != "" {
		query.WriteString(" AND locale = ")
		query.WriteString(addArg(filter.Locale))
	}
	if filter.Status != "" {
		query.WriteString(" AND status = ")
		query.WriteString(addArg(string(filter.Status)))
	}
	if filter.IsActive != nil {
		query.WriteString(" AND is_active = ")
		query.WriteString(addArg(*filter.IsActive))
	}

	query.WriteString(" ORDER BY code ASC, channel ASC, locale ASC, version DESC")

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	query.WriteString(" LIMIT ")
	query.WriteString(addArg(limit))

	if filter.Offset > 0 {
		query.WriteString(" OFFSET ")
		query.WriteString(addArg(filter.Offset))
	}

	rows, err := r.db.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := make([]domain.NotificationTemplate, 0)
	for rows.Next() {
		var template domain.NotificationTemplate
		if err := rows.Scan(templateScanDest(&template)...); err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}

	return templates, rows.Err()
}

func (r *TemplateRepository) Update(ctx context.Context, template *domain.NotificationTemplate) error {
	availableVariables, err := encodeVariables(template.AvailableVariables)
	if err != nil {
		return err
	}
	samplePayload, err := encodeMap(template.SamplePayload)
	if err != nil {
		return err
	}

	err = r.db.QueryRow(ctx, `
		UPDATE notification_templates
		SET
			name = $2,
			description = NULLIF($3, ''),
			locale = $4,
			subject_template = NULLIF($5, ''),
			body_template = $6,
			available_variables = $7::jsonb,
			sample_payload = $8::jsonb,
			is_active = $9,
			updated_by = NULLIF($10, '')::uuid,
			updated_at = now()
		WHERE id = $1
			AND deleted_at IS NULL
		RETURNING `+templateSelectColumns,
		template.ID,
		template.Name,
		template.Description,
		template.Locale,
		template.SubjectTemplate,
		template.BodyTemplate,
		string(availableVariables),
		string(samplePayload),
		template.IsActive,
		template.UpdatedBy,
	).Scan(templateScanDest(template)...)
	if err != nil {
		return err
	}

	return nil
}

func (r *TemplateRepository) SoftDelete(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE notification_templates
		SET deleted_at = COALESCE(deleted_at, now()), updated_at = now()
		WHERE id = $1
			AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *TemplateRepository) Activate(ctx context.Context, id string) error {
	return r.setStatus(ctx, id, domain.TemplateStatusActive, true)
}

func (r *TemplateRepository) Deactivate(ctx context.Context, id string) error {
	return r.setStatus(ctx, id, domain.TemplateStatusInactive, false)
}

func (r *TemplateRepository) Archive(ctx context.Context, id string) error {
	return r.setStatus(ctx, id, domain.TemplateStatusArchived, false)
}

func (r *TemplateRepository) setStatus(ctx context.Context, id string, status domain.TemplateStatus, isActive bool) error {
	result, err := r.db.Exec(ctx, `
		UPDATE notification_templates
		SET status = $2, is_active = $3, updated_at = now()
		WHERE id = $1
			AND deleted_at IS NULL
	`, id, string(status), isActive)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func templateScanDest(template *domain.NotificationTemplate) []any {
	var deletedAt sql.NullTime

	return []any{
		&template.ID,
		&template.Code,
		&template.Name,
		&template.Description,
		(*channelScanner)(&template.Channel),
		&template.Locale,
		&template.SubjectTemplate,
		&template.BodyTemplate,
		(*variablesScanner)(&template.AvailableVariables),
		(*mapScanner)(&template.SamplePayload),
		(*templateStatusScanner)(&template.Status),
		&template.IsSystem,
		&template.IsActive,
		&template.Version,
		&template.CreatedBy,
		&template.UpdatedBy,
		&template.CreatedAt,
		&template.UpdatedAt,
		&nullableTimeScanner{value: &deletedAt, assign: &template.DeletedAt},
	}
}

type channelScanner domain.Channel

func (s *channelScanner) Scan(value any) error {
	str, err := scanString(value)
	if err != nil {
		return err
	}
	*s = channelScanner(domain.Channel(str))
	return nil
}

type templateStatusScanner domain.TemplateStatus

func (s *templateStatusScanner) Scan(value any) error {
	str, err := scanString(value)
	if err != nil {
		return err
	}
	*s = templateStatusScanner(domain.TemplateStatus(str))
	return nil
}

type variablesScanner []domain.NotificationVariable

func (s *variablesScanner) Scan(value any) error {
	data, err := scanBytes(value)
	if err != nil {
		return err
	}
	var variables []domain.NotificationVariable
	if len(data) > 0 {
		if err := json.Unmarshal(data, &variables); err != nil {
			return err
		}
	}
	*s = variables
	return nil
}

type mapScanner map[string]any

func (s *mapScanner) Scan(value any) error {
	data, err := scanBytes(value)
	if err != nil {
		return err
	}
	payload := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
	}
	*s = payload
	return nil
}

type nullableTimeScanner struct {
	value  *sql.NullTime
	assign **time.Time
}

func (s *nullableTimeScanner) Scan(value any) error {
	if err := s.value.Scan(value); err != nil {
		return err
	}
	if s.value.Valid {
		*s.assign = &s.value.Time
		return nil
	}
	*s.assign = nil
	return nil
}

func encodeVariables(variables []domain.NotificationVariable) ([]byte, error) {
	if variables == nil {
		variables = []domain.NotificationVariable{}
	}
	return json.Marshal(variables)
}

func encodeMap(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

func scanString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return "", fmt.Errorf("unsupported string scan type %T", value)
	}
}

func scanBytes(value any) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("unsupported json scan type %T", value)
	}
}
