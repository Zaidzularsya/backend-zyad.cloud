package service

import (
	"time"

	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
)

func organizationResponse(organization model.Organization) dto.OrganizationResponse {
	return dto.OrganizationResponse{
		ID:            organization.ID,
		Type:          string(organization.Type),
		Slug:          organization.Slug,
		Name:          organization.Name,
		Status:        string(organization.Status),
		Timezone:      organization.Timezone,
		Locale:        organization.Locale,
		Region:        organization.Region,
		DataPlacement: string(organization.DataPlacement),
		Metadata:      organization.Metadata,
		CreatedAt:     organization.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     organization.UpdatedAt.UTC().Format(time.RFC3339),
		DeletedAt:     formatOrganizationTime(organization.DeletedAt),
	}
}
