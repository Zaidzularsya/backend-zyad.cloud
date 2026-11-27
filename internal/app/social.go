package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/modules/social"
	socialdomain "zyad.cloud/internal/modules/social/domain"
	socialhandler "zyad.cloud/internal/modules/social/handler"
	socialrepo "zyad.cloud/internal/modules/social/repository"
	socialservice "zyad.cloud/internal/modules/social/service"
	userdto "zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/redis"
)

// socialModule is what the composition root needs from the social module.
type socialModule struct {
	Attribution *socialservice.AttributionService
	Relations   *socialservice.RelationsService
	Handlers    social.Handlers
}

func buildSocial(db *database.Pool, redisClient *redis.Client, shareURLBase string) socialModule {
	attribution := socialservice.NewAttributionService(socialrepo.NewAttributionRepository(db), shareURLBase)
	relations := socialservice.NewRelationsService(
		socialCheckoutRelations{db: db},
		socialClaimedLinks{db: db},
		socialContactStatus{
			orgs: organizationrepo.NewOrganizationRepository(db),
			tx:   database.NewSharedTenantTransactor(db),
		},
	)
	return socialModule{
		Attribution: attribution,
		Relations:   relations,
		Handlers: social.Handlers{
			Public: socialhandler.NewPublicHandler(attribution, redisClient),
			Me:     socialhandler.NewMeHandler(attribution, relations),
		},
	}
}

// socialSignupHooks implements user/service.SignupHooks. Attribution is
// best-effort: the user already exists, so failures are logged and swallowed.
type socialSignupHooks struct {
	attribution *socialservice.AttributionService
	log         *slog.Logger
}

func (h socialSignupHooks) OnUserRegistered(ctx context.Context, userID string, in *userdto.AttributionInput) {
	if in == nil {
		return
	}
	err := h.attribution.RecordSignup(ctx, userID, &socialdomain.AttributionInput{
		Code:        in.Code,
		Channel:     in.Channel,
		PostID:      in.PostID,
		FirstSeenAt: in.FirstSeenAt,
	})
	if err != nil {
		h.log.WarnContext(ctx, "record signup attribution failed", "user_id", userID, "error", err)
	}
}

// socialCheckoutRelations: workspaces the user actively owns (customer orgs) that
// a CRM company in some organization links to via crm_companies.tenant_organization_id.
// crm_companies is FORCE RLS and the company lives in the SELLER org, so the lookup
// uses the narrow app.crm_workspace_link_lookup context (migration 000167), one
// workspace at a time, with app.organization_id left empty — same pattern as
// billSellers.SellerOf / companyRepository.FindWorkspaceLinkAnyOrg.
type socialCheckoutRelations struct {
	db *database.Pool
}

func (s socialCheckoutRelations) CheckoutRelations(ctx context.Context, userID string) ([]socialservice.CustomerRelation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT o.id::text
		FROM organization_memberships m
		JOIN organizations o ON o.id = m.organization_id AND o.deleted_at IS NULL
		WHERE m.user_id = $1::uuid AND m.is_owner AND m.status = 'active' AND m.removed_at IS NULL
			AND o.type = 'customer'
		ORDER BY m.created_at, o.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list owned workspaces: %w", err)
	}
	workspaces, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("scan owned workspaces: %w", err)
	}
	if len(workspaces) == 0 {
		return nil, nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())

	relations := make([]socialservice.CustomerRelation, 0, len(workspaces))
	for _, workspaceID := range workspaces {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.crm_workspace_link_lookup', $1, true)", workspaceID); err != nil {
			return nil, err
		}
		var sellerID, sellerName string
		err := tx.QueryRow(ctx, `
			SELECT o.id::text, o.name
			FROM crm_companies c
			JOIN organizations o ON o.id = c.organization_id AND o.deleted_at IS NULL
			WHERE c.tenant_organization_id = $1::uuid AND c.deleted_at IS NULL
			ORDER BY c.created_at, c.id
			LIMIT 1`, workspaceID).Scan(&sellerID, &sellerName)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("find seller of workspace %s: %w", workspaceID, err)
		}
		relations = append(relations, socialservice.CustomerRelation{
			OrganizationID:   sellerID,
			OrganizationName: sellerName,
			Source:           socialservice.RelationSourceCheckout,
		})
	}
	return relations, nil
}

// socialClaimedLinks reads customer_links of the user. The table is global (no RLS,
// ruling R4); the query is keyed by user id, never by client-supplied organization.
type socialClaimedLinks struct {
	db *database.Pool
}

func (s socialClaimedLinks) ClaimedLinks(ctx context.Context, userID string) ([]socialservice.ClaimedLink, error) {
	rows, err := s.db.Query(ctx, `
		SELECT l.id::text, l.organization_id::text, o.name, l.crm_contact_id::text, l.status
		FROM customer_links l
		JOIN organizations o ON o.id = l.organization_id AND o.deleted_at IS NULL
		WHERE l.user_id = $1::uuid AND l.status = 'active'
		ORDER BY l.activated_at NULLS LAST, l.created_at, l.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list customer links: %w", err)
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (socialservice.ClaimedLink, error) {
		var l socialservice.ClaimedLink
		err := row.Scan(&l.ID, &l.OrganizationID, &l.OrganizationName, &l.ContactID, &l.Status)
		return l, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan customer links: %w", err)
	}
	return links, nil
}

// socialContactStatus checks a CRM contact inside its own tenant transaction
// (crm_contacts is FORCE RLS). The tenant context is internal: built from the
// organization row, not from any request input.
type socialContactStatus struct {
	orgs *organizationrepo.OrganizationRepository
	tx   *database.TenantTransactor
}

func (s socialContactStatus) ContactActive(ctx context.Context, organizationID, contactID string) (bool, error) {
	org, err := s.orgs.FindByID(ctx, organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load organization: %w", err)
	}
	tc, err := internalTenantContext(org)
	if err != nil {
		return false, err
	}
	tx, err := s.tx.BeginForTenant(ctx, tc)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())

	var active bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM crm_contacts
			WHERE id = $1::uuid AND organization_id = $2::uuid AND deleted_at IS NULL
		)`, contactID, organizationID).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("check crm contact: %w", err)
	}
	return active, nil
}

func internalTenantContext(org model.Organization) (coretenant.Context, error) {
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     org.ID,
		OrganizationSlug:   org.Slug,
		OrganizationType:   org.Type,
		OrganizationStatus: org.Status,
		ResolutionSource:   coretenant.ResolutionSourceInternal,
		DataPlacement:      org.DataPlacement,
	})
	if err != nil {
		return coretenant.Context{}, fmt.Errorf("build tenant context for %s: %w", org.ID, err)
	}
	return tc, nil
}
