package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
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
	Claims      *socialservice.ClaimService
	Invites     *socialservice.InviteService
	StaffLinks  *socialservice.StaffLinkService
	Settings    *socialservice.SettingsService
	Handlers    social.Handlers
}

// socialConfig carries what the social module needs from config and other modules.
type socialConfig struct {
	ShareURLBase string
	ClaimURLBase string
	AppName      string
	Locale       string
	MaxAttempts  int
	Publisher    notificationPublisher
	Verifier     socialservice.EmailVerifier
}

func buildSocial(db *database.Pool, redisClient *redis.Client, cfg socialConfig) socialModule {
	shareURLBase := cfg.ShareURLBase
	attribution := socialservice.NewAttributionService(socialrepo.NewAttributionRepository(db), shareURLBase)
	tenants := socialTenantScope{
		orgs: organizationrepo.NewOrganizationRepository(db),
		tx:   database.NewSharedTenantTransactor(db),
	}
	relations := socialservice.NewRelationsService(
		socialCheckoutRelations{db: db},
		socialClaimedLinks{db: db},
		socialContactStatus{tenants: tenants},
	)
	settings := socialservice.NewSettingsService(socialrepo.NewSettingsRepository(db))
	contacts := socialContactDirectory{db: db, tenants: tenants}
	links := socialrepo.NewCustomerLinkRepository(db)
	claims := socialservice.NewClaimService(contacts, links, settings, attribution)
	invites := socialservice.NewInviteService(
		contacts,
		links,
		socialrepo.NewInviteRepository(db),
		socialInviteSender{publisher: cfg.Publisher, appName: cfg.AppName, locale: cfg.Locale, maxAttempts: cfg.MaxAttempts},
		cfg.Verifier,
		attribution,
		cfg.ClaimURLBase,
	)
	staffLinks := socialservice.NewStaffLinkService(contacts, links, socialUserDirectory{db: db}, claims)
	return socialModule{
		Attribution: attribution,
		Relations:   relations,
		Claims:      claims,
		Invites:     invites,
		StaffLinks:  staffLinks,
		Settings:    settings,
		Handlers: social.Handlers{
			Public: socialhandler.NewPublicHandler(attribution, redisClient),
			Me:     socialhandler.NewMeHandler(attribution, relations, claims, invites),
			Tenant: socialhandler.NewTenantHandler(settings, invites, staffLinks, redisClient),
		},
	}
}

// socialUserDirectory looks users up by email for staff link requests, without
// importing the user module.
type socialUserDirectory struct {
	db *database.Pool
}

func (d socialUserDirectory) FindUserIDByEmail(ctx context.Context, email string) (string, bool, error) {
	var id string
	err := d.db.QueryRow(ctx, `
		SELECT id::text FROM users
		WHERE lower(btrim(email)) = lower(btrim($1)) AND deleted_at IS NULL
		ORDER BY created_at, id
		LIMIT 1`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find user by email: %w", err)
	}
	return id, true, nil
}

// socialInviteSender queues the claim invite email through the notification
// outbox. The recipient is an external address (the contact may have no account yet).
type socialInviteSender struct {
	publisher   notificationPublisher
	appName     string
	locale      string
	maxAttempts int
}

const eventSocialCustomerClaimInvite = "social.customer_claim_invite"

func (s socialInviteSender) SendClaimInvite(ctx context.Context, msg socialservice.ClaimInviteEmail) error {
	if s.publisher == nil {
		return errors.New("social: notification publisher is not configured")
	}
	orgName := strings.TrimSpace(msg.OrganizationName)
	if orgName == "" {
		orgName = "-"
	}
	appName := strings.TrimSpace(s.appName)
	if appName == "" {
		appName = "Zyad Cloud"
	}
	contactName := strings.TrimSpace(msg.ContactName)
	if contactName == "" {
		contactName = "Pelanggan"
	}
	_, err := s.publisher.Publish(ctx, notificationpublisher.Event{
		Type:           eventSocialCustomerClaimInvite,
		OrganizationID: msg.OrganizationID,
		Recipient:      notificationdomain.NotificationRecipient{Type: "external", Name: contactName, Email: msg.Email},
		Payload: map[string]any{
			"app_name":          appName,
			"organization_name": orgName,
			"contact_name":      contactName,
			"claim_url":         msg.ClaimURL,
			"expired_at":        msg.ExpiresAt.In(businesstime.Location()).Format("02-01-2006 15:04") + " WIB",
		},
		Locale:      s.locale,
		MaxAttempts: s.maxAttempts,
	})
	if err != nil {
		return fmt.Errorf("publish claim invite email: %w", err)
	}
	return nil
}

// socialEmailVerifier adapts the user auth repository (ruling R2).
type socialEmailVerifier struct {
	users interface {
		MarkEmailVerified(ctx context.Context, userID string, at time.Time) error
	}
}

func (v socialEmailVerifier) MarkEmailVerified(ctx context.Context, userID string, at time.Time) error {
	return v.users.MarkEmailVerified(ctx, userID, at)
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

// socialTenantScope opens a transaction inside one organization's RLS scope for
// cross-tenant reads (crm_contacts, receivable_accounts and
// mikrotik_secret_onboardings are FORCE RLS). The tenant context is internal:
// built from the organization row, never from request input.
type socialTenantScope struct {
	orgs *organizationrepo.OrganizationRepository
	tx   *database.TenantTransactor
}

// begin returns found=false when the organization does not exist (or is archived).
func (s socialTenantScope) begin(ctx context.Context, organizationID string) (pgx.Tx, model.Organization, bool, error) {
	org, err := s.orgs.FindByID(ctx, organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.Organization{}, false, nil
	}
	if err != nil {
		return nil, model.Organization{}, false, fmt.Errorf("load organization: %w", err)
	}
	tc, err := internalTenantContext(org)
	if err != nil {
		return nil, model.Organization{}, false, err
	}
	tx, err := s.tx.BeginForTenant(ctx, tc)
	if err != nil {
		return nil, model.Organization{}, false, err
	}
	return tx, org, true, nil
}

// socialContactStatus checks a CRM contact inside its own tenant transaction.
type socialContactStatus struct {
	tenants socialTenantScope
}

func (s socialContactStatus) ContactActive(ctx context.Context, organizationID, contactID string) (bool, error) {
	tx, _, found, err := s.tenants.begin(ctx, organizationID)
	if err != nil || !found {
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

// socialContactDirectory implements socialservice.ContactDirectory.
//
// Candidates come from the narrow cross-org email lookup on crm_contacts
// (app.crm_contact_email_lookup, migration 000189: SELECT only, non-deleted
// contacts with exactly that normalized email). Each candidate is then
// re-checked inside its own organization's tenant transaction, where the
// customer evidence lives: an undeleted receivable account sourced from the
// contact, or a MikroTik secret onboarding confirmed/paired to it.
type socialContactDirectory struct {
	db      *database.Pool
	tenants socialTenantScope
}

type socialContactCandidate struct {
	organizationID string
	contactID      string
}

func (d socialContactDirectory) FindClaimableByEmail(ctx context.Context, email string) ([]socialservice.ClaimableContact, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, nil
	}
	candidates, err := d.emailCandidates(ctx, email)
	if err != nil {
		return nil, err
	}
	out := make([]socialservice.ClaimableContact, 0, len(candidates))
	for _, cand := range candidates {
		contact, ok, err := d.IsClaimable(ctx, cand.organizationID, cand.contactID)
		if err != nil {
			return nil, err
		}
		if ok && strings.ToLower(strings.TrimSpace(contact.Email)) == email {
			out = append(out, contact)
		}
	}
	return out, nil
}

func (d socialContactDirectory) emailCandidates(ctx context.Context, email string) ([]socialContactCandidate, error) {
	tx, err := d.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT set_config('app.crm_contact_email_lookup', $1, true)", email); err != nil {
		return nil, fmt.Errorf("set crm contact email lookup: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT c.organization_id::text, c.id::text
		FROM crm_contacts c
		JOIN organizations o ON o.id = c.organization_id AND o.deleted_at IS NULL AND o.status = 'active'
		WHERE c.deleted_at IS NULL AND lower(btrim(c.email)) = $1
		ORDER BY o.name, c.created_at, c.id`, email)
	if err != nil {
		return nil, fmt.Errorf("lookup crm contacts by email: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (socialContactCandidate, error) {
		var c socialContactCandidate
		err := row.Scan(&c.organizationID, &c.contactID)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan crm contact candidates: %w", err)
	}
	return candidates, nil
}

func (d socialContactDirectory) IsClaimable(ctx context.Context, organizationID, contactID string) (socialservice.ClaimableContact, bool, error) {
	tx, org, found, err := d.tenants.begin(ctx, organizationID)
	if err != nil || !found {
		return socialservice.ClaimableContact{}, false, err
	}
	defer tx.Rollback(context.Background())
	if org.Status != coretenant.OrganizationStatusActive {
		return socialservice.ClaimableContact{}, false, nil
	}

	contact := socialservice.ClaimableContact{OrganizationID: org.ID, OrganizationName: org.Name, ContactID: contactID}
	var customer bool
	err = tx.QueryRow(ctx, `
		SELECT btrim(c.first_name || ' ' || COALESCE(c.last_name, '')), COALESCE(c.email, ''),
			EXISTS (
				SELECT 1 FROM receivable_accounts a
				WHERE a.organization_id = c.organization_id AND a.source_type = 'crm_contact'
					AND a.source_id = c.id AND a.deleted_at IS NULL
			) OR EXISTS (
				SELECT 1 FROM mikrotik_secret_onboardings m
				WHERE m.organization_id = c.organization_id AND m.contact_id = c.id
					AND m.stage IN ('confirmed', 'paired')
			)
		FROM crm_contacts c
		WHERE c.id = $1::uuid AND c.organization_id = $2::uuid AND c.deleted_at IS NULL`,
		contactID, organizationID).Scan(&contact.ContactName, &contact.Email, &customer)
	if errors.Is(err, pgx.ErrNoRows) {
		return socialservice.ClaimableContact{}, false, nil
	}
	if err != nil {
		return socialservice.ClaimableContact{}, false, fmt.Errorf("check claimable crm contact: %w", err)
	}
	if !customer {
		return socialservice.ClaimableContact{}, false, nil
	}
	return contact, true, nil
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
