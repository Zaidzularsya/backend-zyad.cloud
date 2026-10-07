package app

import (
	"context"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	crmservice "zyad.cloud/internal/modules/crm/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// workspaceDirectory adalah adapter tipis CRM → organization untuk memeriksa dan meringkas workspace tertaut.
type workspaceDirectory struct {
	orgs *organizationrepo.OrganizationRepository
}

func (d workspaceDirectory) FindWorkspace(ctx context.Context, id string) (crmservice.WorkspaceInfo, error) {
	if !uuidPattern.MatchString(id) {
		return crmservice.WorkspaceInfo{}, crmservice.ErrWorkspaceNotFound
	}
	org, err := d.orgs.FindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmservice.WorkspaceInfo{}, crmservice.ErrWorkspaceNotFound
	}
	if err != nil {
		return crmservice.WorkspaceInfo{}, err
	}
	return crmservice.WorkspaceInfo{
		ID: org.ID, Name: org.Name, Slug: org.Slug, Status: string(org.Status),
		Customer: org.Type == coretenant.OrganizationTypeCustomer,
	}, nil
}
