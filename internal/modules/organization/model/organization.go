package model

import (
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
)

type Organization struct {
	ID            string
	Type          coretenant.OrganizationType
	Slug          string
	Name          string
	Status        coretenant.OrganizationStatus
	Timezone      string
	Locale        string
	Region        string
	DataPlacement coretenant.DataPlacement
	Metadata      map[string]any
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

func (o Organization) IsPlatform() bool {
	return o.Type == coretenant.OrganizationTypePlatform
}

func (o Organization) IsActive() bool {
	return o.DeletedAt == nil && o.Status == coretenant.OrganizationStatusActive
}
