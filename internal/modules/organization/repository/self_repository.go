package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

var ErrNoOrganizationChanges = errors.New("organization update has no changes")

type SelfRepository struct {
	db *database.Pool
}

type UpdateCurrentOrganizationParams struct {
	Name         *string
	Timezone     *string
	Locale       *string
	Region       *string
	Metadata     *map[string]any
	ActorUserID  string
	MembershipID string
	ChangedAt    time.Time
}

func NewSelfRepository(db *database.Pool) *SelfRepository {
	return &SelfRepository{db: db}
}

func (r *SelfRepository) FindByID(
	ctx context.Context,
	organizationID string,
) (model.Organization, error) {
	return NewOrganizationRepository(r.db).FindByID(ctx, organizationID)
}

func (r *SelfRepository) Update(
	ctx context.Context,
	organizationID string,
	params UpdateCurrentOrganizationParams,
) (model.Organization, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Organization{}, err
	}
	defer tx.Rollback(ctx)

	setClauses := make([]string, 0, 6)
	args := make([]any, 0, 7)
	changes := make(map[string]any)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if params.Name != nil {
		value := strings.TrimSpace(*params.Name)
		setClauses = append(setClauses, "name = "+addArg(value))
		changes["name"] = value
	}
	if params.Timezone != nil {
		value := strings.TrimSpace(*params.Timezone)
		setClauses = append(setClauses, "timezone = "+addArg(value))
		changes["timezone"] = value
	}
	if params.Locale != nil {
		value := strings.TrimSpace(*params.Locale)
		setClauses = append(setClauses, "locale = "+addArg(value))
		changes["locale"] = value
	}
	if params.Region != nil {
		value := strings.TrimSpace(*params.Region)
		setClauses = append(
			setClauses,
			"region = NULLIF("+addArg(value)+", '')",
		)
		changes["region"] = value
	}
	if params.Metadata != nil {
		metadata, marshalErr := json.Marshal(*params.Metadata)
		if marshalErr != nil {
			return model.Organization{}, marshalErr
		}
		setClauses = append(
			setClauses,
			"metadata = "+addArg(string(metadata))+"::jsonb",
		)
		changes["metadata"] = *params.Metadata
	}
	if len(setClauses) == 0 {
		return model.Organization{}, ErrNoOrganizationChanges
	}

	setClauses = append(setClauses, "updated_at = "+addArg(params.ChangedAt))
	args = append(args, strings.TrimSpace(organizationID))

	var organization model.Organization
	var metadataBytes []byte
	err = tx.QueryRow(ctx, `
		UPDATE organizations
		SET `+strings.Join(setClauses, ", ")+`
		WHERE id = $`+fmt.Sprint(len(args))+`::uuid
			AND type = 'customer'
			AND status = 'active'
			AND deleted_at IS NULL
		RETURNING `+organizationSelectColumns,
		args...,
	).Scan(organizationScanDest(&organization, &metadataBytes)...)
	if err != nil {
		return model.Organization{}, err
	}
	if err := decodeMetadata(metadataBytes, &organization.Metadata); err != nil {
		return model.Organization{}, err
	}
	if err := insertOrganizationAuditTx(
		ctx,
		tx,
		organization,
		model.Membership{ID: strings.TrimSpace(params.MembershipID)},
		params.ActorUserID,
		"organization_profile_updated",
		changes,
		params.ChangedAt,
	); err != nil {
		return model.Organization{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Organization{}, err
	}
	return organization, nil
}
