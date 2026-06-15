package repository

import (
	"context"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

type PublicHostResolverRepository struct {
	domains       *DomainRepository
	organizations *OrganizationRepository
}

func NewPublicHostResolverRepository(db *database.Pool) *PublicHostResolverRepository {
	return &PublicHostResolverRepository{
		domains:       NewDomainRepository(db),
		organizations: NewOrganizationRepository(db),
	}
}

func (r *PublicHostResolverRepository) ResolveActiveHost(
	ctx context.Context,
	host string,
) (ResolvedDomain, error) {
	return r.domains.ResolveActiveHost(ctx, host)
}

func (r *PublicHostResolverRepository) FindOrganizationByID(
	ctx context.Context,
	organizationID string,
) (model.Organization, error) {
	return r.organizations.FindByID(ctx, organizationID)
}
