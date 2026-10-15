package repository

import (
	"context"
	"errors"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
)

var (
	ErrPlaybookDisabled      = errors.New("lead playbook is disabled for this organization")
	ErrPlaybookAlreadyActive = errors.New("lead already has an active playbook run")
	ErrPlaybookNotApplicable = errors.New("no playbook step applies to the lead's current status")
	ErrPlaybookRunNotActive  = errors.New("playbook run is not active")
)

type CRMSettings struct {
	LeadPlaybookEnabled bool
	UpdatedAt           *time.Time
}

type CRMSettingsRepository interface {
	Get(context.Context, coretenant.Scope) (CRMSettings, error)
	Update(ctx context.Context, scope coretenant.Scope, leadPlaybookEnabled bool, updatedBy string) (CRMSettings, error)
}
