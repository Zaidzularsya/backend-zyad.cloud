package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	crmservice "zyad.cloud/internal/modules/crm/service"
	organizationdto "zyad.cloud/internal/modules/organization/dto"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	userservice "zyad.cloud/internal/modules/user/service"
	"zyad.cloud/internal/platform/database"
)

// tenantCheckoutMetadataKey menandai workspace yang dibuat checkout publik tenant (nilai = id org
// tenant). Dipakai untuk menemukan kembali workspace bila rantai CRM gagal sebelum company tertaut,
// sehingga percobaan ulang tidak membuat workspace kedua.
const tenantCheckoutMetadataKey = "tenant_checkout_organization_id"

// googleBuyerAdapter: AuthService.ResolveGoogleBuyer (tanpa sesi) → crmservice.GoogleBuyerVerifier.
type googleBuyerAdapter struct {
	auth interface {
		ResolveGoogleBuyer(ctx context.Context, idToken string, metadata userservice.LoginHistoryRecord) (userservice.GoogleBuyer, error)
	}
}

func (a googleBuyerAdapter) ResolveGoogleBuyer(ctx context.Context, credential, ip, userAgent string) (crmservice.TenantBuyer, error) {
	b, err := a.auth.ResolveGoogleBuyer(ctx, credential, userservice.LoginHistoryRecord{IPAddress: ip, UserAgent: userAgent})
	if err != nil {
		return crmservice.TenantBuyer{}, err
	}
	return crmservice.TenantBuyer{UserID: b.UserID, Name: b.Name, Email: b.Email}, nil
}

type ownedWorkspaceCreator interface {
	CreateOwnedWorkspace(ctx context.Context, userID string, request organizationdto.CreateWorkspaceRequest, metadata organizationservice.OnboardingMetadata) (string, error)
}

// tenantBuyerWorkspaces: satu workspace per (tenant, pembeli).
type tenantBuyerWorkspaces struct {
	db         *database.Pool
	onboarding ownedWorkspaceCreator
}

// FindForTenant mencari workspace customer yang DIMILIKI user (is_owner, membership aktif) dan
// (a) sudah tertaut ke company milik tenant ini, atau (b) dibuat oleh checkout tenant ini.
// Query lintas-org yang disengaja: kedua cabang memfilter organization_id tenant secara eksplisit.
func (w tenantBuyerWorkspaces) FindForTenant(ctx context.Context, tenantOrganizationID, userID string) (string, bool, error) {
	var id string
	err := w.db.QueryRow(ctx, `
		SELECT o.id::text
		FROM organizations o
		JOIN organization_memberships m ON m.organization_id = o.id
		WHERE m.user_id = $2::uuid
			AND m.status = 'active'
			AND m.is_owner
			AND o.type = 'customer'
			AND o.deleted_at IS NULL
			AND o.id <> $1::uuid
			AND (
				EXISTS (
					SELECT 1 FROM crm_companies c
					WHERE c.organization_id = $1::uuid
						AND c.tenant_organization_id = o.id
						AND c.deleted_at IS NULL
				)
				OR o.metadata ->> '`+tenantCheckoutMetadataKey+`' = $1::text
			)
		ORDER BY o.created_at, o.id
		LIMIT 1`, tenantOrganizationID, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

func (w tenantBuyerWorkspaces) CreateForTenant(ctx context.Context, tenantOrganizationID string, buyer crmservice.TenantBuyer, ip, userAgent string) (string, error) {
	req, err := tenantBuyerWorkspaceRequest(tenantOrganizationID, buyer)
	if err != nil {
		return "", err
	}
	return w.onboarding.CreateOwnedWorkspace(ctx, buyer.UserID, req, organizationservice.OnboardingMetadata{IPAddress: ip, UserAgent: userAgent})
}

// tenantBuyerWorkspaceRequest: nama = nama pembeli (jadi nama company di CRM tenant); slug memakai
// sufiks acak karena satu user boleh punya banyak workspace (satu per tenant).
func tenantBuyerWorkspaceRequest(tenantOrganizationID string, buyer crmservice.TenantBuyer) (organizationdto.CreateWorkspaceRequest, error) {
	name := strings.TrimSpace(buyer.Name)
	if name == "" {
		name = strings.TrimSpace(buyer.Email)
	}
	if r := []rune(name); len(r) > 150 {
		name = string(r[:150])
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return organizationdto.CreateWorkspaceRequest{}, err
	}
	return organizationdto.CreateWorkspaceRequest{
		Name:     name,
		Slug:     temporaryWorkspaceSlug(name, hex.EncodeToString(suffix)),
		Timezone: "Asia/Jakarta",
		Locale:   "id-ID",
		Metadata: map[string]any{tenantCheckoutMetadataKey: tenantOrganizationID},
	}, nil
}

var (
	_ crmservice.GoogleBuyerVerifier   = googleBuyerAdapter{}
	_ crmservice.TenantBuyerWorkspaces = tenantBuyerWorkspaces{}
)
