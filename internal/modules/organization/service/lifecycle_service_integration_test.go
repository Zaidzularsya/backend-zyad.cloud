//go:build integration

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestLifecycleServiceCreateRollbackAndStatusEffectsIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	store := repository.NewLifecycleRepository(db)
	service := NewLifecycleService(store)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("lifecycle"), ".", "-")
	now := time.Now().UTC().Truncate(time.Microsecond)
	service.now = func() time.Time { return now }

	var ownerUserID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status)
		VALUES ('Lifecycle Owner', $1, 'active')
		RETURNING id
	`, fmt.Sprintf("lifecycle-%s@example.test", suffix)).Scan(&ownerUserID); err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `
			DELETE FROM audit_logs
			WHERE organization_id IN (
				SELECT id FROM organizations WHERE slug LIKE $1
			)
		`, "lifecycle-%"+suffix+"%")
		_, _ = db.Exec(cleanupCtx, `
			DELETE FROM sessions
			WHERE user_id = $1
		`, ownerUserID)
		_, _ = db.Exec(cleanupCtx, `
			DELETE FROM organizations
			WHERE slug LIKE $1
		`, "lifecycle-%"+suffix+"%")
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, ownerUserID)
	})

	rollbackSlug := "lifecycle-rollback-" + suffix
	_, err := service.Create(ctx, CreateOrganizationInput{
		Type:        coretenant.OrganizationTypeCustomer,
		Slug:        rollbackSlug,
		Name:        "Rollback Organization",
		OwnerUserID: "11111111-1111-1111-1111-111111111111",
	})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "ORGANIZATION_OWNER_NOT_FOUND" {
		t.Fatalf("Create() invalid owner error = %v", err)
	}
	var rollbackCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM organizations WHERE slug = $1
	`, rollbackSlug).Scan(&rollbackCount); err != nil {
		t.Fatalf("count rollback organization: %v", err)
	}
	if rollbackCount != 0 {
		t.Fatalf("rollback organization count = %d, want 0", rollbackCount)
	}

	bundle, err := service.Create(ctx, CreateOrganizationInput{
		Type:        coretenant.OrganizationTypeCustomer,
		Slug:        "lifecycle-success-" + suffix,
		Name:        "Lifecycle Organization",
		OwnerUserID: ownerUserID,
		ActorUserID: ownerUserID,
		Metadata:    map[string]any{"branding_state": "default"},
		Plan: []repository.UpsertEntitlementParams{
			{
				FeatureKey:      "landing.enabled",
				SourceReference: "internal-plan",
				Limits:          map[string]any{"enabled": true},
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if bundle.Organization.Status != coretenant.OrganizationStatusProvisioning ||
		!bundle.Owner.IsOwner ||
		!bundle.Owner.IsActive() ||
		len(bundle.Entitlements) != 1 ||
		bundle.Entitlements[0].Source != model.EntitlementSourcePlan {
		t.Fatalf("Create() bundle = %#v", bundle)
	}

	var createAuditCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE organization_id = $1
			AND event = 'organization_created'
			AND membership_id = $2
	`, bundle.Organization.ID, bundle.Owner.ID).Scan(&createAuditCount); err != nil {
		t.Fatalf("count create audit: %v", err)
	}
	if createAuditCount != 1 {
		t.Fatalf("create audit count = %d, want 1", createAuditCount)
	}

	activeOrganization, err := service.ChangeStatus(ctx, bundle.Organization, ChangeOrganizationStatusInput{
		OrganizationID: bundle.Organization.ID,
		Status:         coretenant.OrganizationStatusActive,
		Reason:         "provisioning completed",
		ActorUserID:    ownerUserID,
	})
	if err != nil {
		t.Fatalf("activate organization: %v", err)
	}

	var sessionID string
	if err := db.QueryRow(ctx, `
		INSERT INTO sessions (
			user_id,
			refresh_token_hash,
			expires_at,
			active_organization_id,
			active_membership_id,
			active_membership_version
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, ownerUserID, "lifecycle-session-"+suffix, now.Add(time.Hour),
		activeOrganization.ID, bundle.Owner.ID, bundle.Owner.Version).Scan(&sessionID); err != nil {
		t.Fatalf("create active organization session: %v", err)
	}

	service.now = func() time.Time { return now.Add(time.Minute) }
	suspendedOrganization, err := service.ChangeStatus(ctx, activeOrganization, ChangeOrganizationStatusInput{
		OrganizationID: activeOrganization.ID,
		Status:         coretenant.OrganizationStatusSuspended,
		Reason:         "billing hold",
		ActorUserID:    ownerUserID,
	})
	if err != nil {
		t.Fatalf("suspend organization: %v", err)
	}
	if suspendedOrganization.Status != coretenant.OrganizationStatusSuspended {
		t.Fatalf("suspended status = %q", suspendedOrganization.Status)
	}

	var revokedAt *time.Time
	if err := db.QueryRow(ctx, `
		SELECT revoked_at FROM sessions WHERE id = $1
	`, sessionID).Scan(&revokedAt); err != nil {
		t.Fatalf("read revoked session: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("expected organization session to be revoked")
	}

	var statusAuditCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE organization_id = $1
			AND event = 'organization_status_changed'
	`, activeOrganization.ID).Scan(&statusAuditCount); err != nil {
		t.Fatalf("count status audits: %v", err)
	}
	if statusAuditCount != 2 {
		t.Fatalf("status audit count = %d, want 2", statusAuditCount)
	}
}
