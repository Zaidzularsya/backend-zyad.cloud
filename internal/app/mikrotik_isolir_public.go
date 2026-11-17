package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	mailboxdomain "zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mikrotik/service"
	wadomain "zyad.cloud/internal/modules/whatsapp/domain"
)

// publicLookupMax mirrors the repository cap: enough rows to tell 0, 1 and ambiguous.
const publicLookupMax = 5

// AccountsByPhone matches normalized phones in one organization-scoped query.
// normalize_phone_id (migration 000125) is the SQL twin of phone.NormalizeID.
func (r mikrotikReceivableReader) AccountsByPhone(ctx context.Context, scope coretenant.Scope, normalizedPhone string) ([]service.AccountRef, error) {
	out := []service.AccountRef{}
	err := r.scopedRead(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, name, COALESCE(phone, ''), COALESCE(email, '')
			FROM receivable_accounts
			WHERE organization_id = $1 AND deleted_at IS NULL
			  AND normalize_phone_id(phone) = $2
			ORDER BY id
			LIMIT $3`, scope.OrganizationID(), normalizedPhone, publicLookupMax)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a service.AccountRef
			if err := rows.Scan(&a.ID, &a.Name, &a.Phone, &a.Email); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

type mikrotikPublicNames interface {
	AccountIDsByName(ctx context.Context, scope coretenant.Scope, name string) ([]string, error)
}

type mikrotikPublicPhones interface {
	AccountsByPhone(ctx context.Context, scope coretenant.Scope, normalizedPhone string) ([]service.AccountRef, error)
}

type mikrotikPublicFeatures interface {
	RequireFeature(ctx context.Context, organizationID, featureKey string) error
}

// mikrotikPublicAdapter satisfies the service ports of the public isolir page
// (PublicDirectory, PublicPayLinks, PublicTenantProfile, PublicFeatureGate).
type mikrotikPublicAdapter struct {
	names    mikrotikPublicNames
	phones   mikrotikPublicPhones
	accounts mikrotikNotifAccounts
	settings mikrotikNotifSettings
	links    payLinker
	issuer   mikrotikNotifIssuer
	features mikrotikPublicFeatures
	// sessions and mailboxes supply the CS contact fallback (nil = no fallback).
	sessions  mikrotikContactSessions
	mailboxes mikrotikContactMailboxes
}

// mikrotikContactSessions lists the tenant's WhatsApp sessions.
type mikrotikContactSessions interface {
	List(ctx context.Context, scope coretenant.Scope) ([]wadomain.Session, error)
}

// mikrotikContactMailboxes lists one user's connected mailboxes.
type mikrotikContactMailboxes interface {
	ListByUser(ctx context.Context, scope coretenant.Scope, userID string) ([]mailboxdomain.Mailbox, error)
}

var (
	_ service.PublicDirectory     = mikrotikPublicAdapter{}
	_ service.PublicPayLinks      = mikrotikPublicAdapter{}
	_ service.PublicTenantProfile = mikrotikPublicAdapter{}
	_ service.PublicFeatureGate   = mikrotikPublicAdapter{}
)

func newMikrotikPublicAdapter(names mikrotikPublicNames, phones mikrotikPublicPhones, accounts mikrotikNotifAccounts,
	settings mikrotikNotifSettings, links payLinker, issuer mikrotikNotifIssuer, features mikrotikPublicFeatures,
	sessions mikrotikContactSessions, mailboxes mikrotikContactMailboxes) mikrotikPublicAdapter {
	return mikrotikPublicAdapter{names: names, phones: phones, accounts: accounts, settings: settings, links: links,
		issuer: issuer, features: features, sessions: sessions, mailboxes: mailboxes}
}

// AccountsByUsername: one DISTINCT scoped id query (<= 5 rows), then at most 5 account reads.
func (a mikrotikPublicAdapter) AccountsByUsername(ctx context.Context, scope coretenant.Scope, username string) ([]service.AccountRef, error) {
	ids, err := a.names.AccountIDsByName(ctx, scope, username)
	if err != nil {
		return nil, err
	}
	out := make([]service.AccountRef, 0, len(ids))
	for _, id := range ids {
		acc, err := a.accounts.Get(ctx, scope, id)
		if err != nil {
			return nil, err
		}
		out = append(out, accountRef(acc))
	}
	return out, nil
}

func (a mikrotikPublicAdapter) AccountsByPhone(ctx context.Context, scope coretenant.Scope, normalizedPhone string) ([]service.AccountRef, error) {
	return a.phones.AccountsByPhone(ctx, scope, normalizedPhone)
}

// onlinePaymentFeatureKey gates online payment on the public invoice page
// (receivable's onlinePaymentAllowed, plain-invoice case).
const onlinePaymentFeatureKey = "receivable.online_payment"

// PayLink returns "" without error when the tenant cannot take the payment
// online (no receivable.online_payment) or has no default sender (no link can
// be built); the page then shows the tenant contact instead of Bayar.
func (a mikrotikPublicAdapter) PayLink(ctx context.Context, scope coretenant.Scope, invoiceID string) (string, error) {
	if err := a.features.RequireFeature(ctx, scope.OrganizationID(), onlinePaymentFeatureKey); err != nil {
		if isFeatureNotEnabled(err) {
			return "", nil
		}
		return "", err
	}
	settings, err := a.settings.Get(ctx, scope)
	if err != nil {
		return "", err
	}
	sender := strings.TrimSpace(settings.DefaultSenderUserID)
	if sender == "" {
		return "", nil
	}
	link, _, err := a.links.Link(ctx, scope, invoiceID, sender)
	if err != nil {
		return "", err
	}
	return link, nil
}

// Profile is the tenant's customer-service contact (activation gate and the public isolir page).
// The branding contact wins; an empty phone or email falls back to the channels that already
// send the invoices: the default working WhatsApp session and the default sender's active
// mailbox (prod finding T13-E1: no current UI fills the branding contact). The fallback is best
// effort: a lookup error leaves that field empty.
func (a mikrotikPublicAdapter) Profile(ctx context.Context, scope coretenant.Scope) (service.TenantContact, error) {
	party, err := a.issuer.OrganizationProfile(ctx, scope)
	if err != nil {
		return service.TenantContact{}, err
	}
	contact := service.TenantContact{Name: party.Name, Phone: party.Phone, Email: party.Email}
	if strings.TrimSpace(contact.Phone) == "" {
		contact.Phone = a.sessionPhone(ctx, scope)
	}
	if strings.TrimSpace(contact.Email) == "" {
		contact.Email = a.senderEmail(ctx, scope)
	}
	return contact, nil
}

// sessionPhone: the default working session's number, else the first working one.
func (a mikrotikPublicAdapter) sessionPhone(ctx context.Context, scope coretenant.Scope) string {
	if a.sessions == nil {
		return ""
	}
	sessions, err := a.sessions.List(ctx, scope)
	if err != nil {
		slog.WarnContext(ctx, "mikrotik: list whatsapp sessions for cs contact failed", "error", err)
		return ""
	}
	phone := ""
	for _, s := range sessions {
		if s.Status != wadomain.SessionStatusWorking || strings.TrimSpace(s.Phone) == "" {
			continue
		}
		if s.IsDefault {
			return s.Phone
		}
		if phone == "" {
			phone = s.Phone
		}
	}
	return phone
}

// senderEmail: the first active mailbox of the receivable default sender.
func (a mikrotikPublicAdapter) senderEmail(ctx context.Context, scope coretenant.Scope) string {
	if a.mailboxes == nil || a.settings == nil {
		return ""
	}
	settings, err := a.settings.Get(ctx, scope)
	if err != nil {
		slog.WarnContext(ctx, "mikrotik: read receivable settings for cs contact failed", "error", err)
		return ""
	}
	sender := strings.TrimSpace(settings.DefaultSenderUserID)
	if sender == "" {
		return ""
	}
	list, err := a.mailboxes.ListByUser(ctx, scope, sender)
	if err != nil {
		slog.WarnContext(ctx, "mikrotik: list mailboxes for cs contact failed", "error", err)
		return ""
	}
	for _, mb := range list {
		if mb.Status == mailboxdomain.MailboxStatusActive && strings.TrimSpace(mb.EmailAddress) != "" {
			return mb.EmailAddress
		}
	}
	return ""
}

// Enabled reports the plain entitlement denial as (false, nil); other errors surface.
func (a mikrotikPublicAdapter) Enabled(ctx context.Context, scope coretenant.Scope) (bool, error) {
	err := a.features.RequireFeature(ctx, scope.OrganizationID(), mikrotikFeatureKey)
	if err == nil {
		return true, nil
	}
	if isFeatureNotEnabled(err) {
		return false, nil
	}
	return false, err
}
