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

func TestAuthenticatedResolverSessionAndHeaderIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("resolver"), ".", "-")

	organizationRepo := repository.NewOrganizationRepository(db)
	membershipRepo := repository.NewMembershipRepository(db)
	resolver := NewAuthenticatedResolver(
		repository.NewAuthenticatedResolverRepository(db),
	)

	organization, err := organizationRepo.Create(ctx, repository.CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "resolver-" + suffix,
		Name:          "Resolver Integration Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status)
		VALUES ('Resolver User', $1, 'active')
		RETURNING id
	`, fmt.Sprintf("resolver-%s@example.test", suffix)).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now().UTC()
	membership, err := membershipRepo.Create(ctx, repository.CreateMembershipParams{
		OrganizationID: organization.ID,
		UserID:         userID,
		Status:         model.MembershipStatusActive,
		AcceptedAt:     &now,
	})
	if err != nil {
		t.Fatalf("create membership: %v", err)
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
		VALUES ($1, $2, now() + interval '1 day', $3, $4, $5)
		RETURNING id
	`, userID, "resolver-session-"+suffix, organization.ID, membership.ID,
		membership.Version).Scan(&sessionID); err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM sessions WHERE id = $1`, sessionID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_memberships WHERE id = $1`, membership.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})

	tenantContext, ok, err := resolver.ResolveAuthenticatedOrganization(
		ctx,
		userID,
		sessionID,
		"",
		"app.example.test",
	)
	if err != nil {
		t.Fatalf("resolve session context: %v", err)
	}
	if !ok || tenantContext.OrganizationID() != organization.ID ||
		tenantContext.ResolutionSource() != coretenant.ResolutionSourceSession ||
		tenantContext.MembershipVersion() != membership.Version {
		t.Fatalf("session tenant context = %#v, %v", tenantContext, ok)
	}

	tenantContext, ok, err = resolver.ResolveAuthenticatedOrganization(
		ctx,
		userID,
		sessionID,
		organization.ID,
		"app.example.test",
	)
	if err != nil {
		t.Fatalf("resolve header context: %v", err)
	}
	if !ok || tenantContext.ResolutionSource() != coretenant.ResolutionSourceHeader {
		t.Fatalf("header tenant context = %#v, %v", tenantContext, ok)
	}

	if _, err := db.Exec(ctx, `
		UPDATE organization_memberships
		SET version = version + 1
		WHERE id = $1
	`, membership.ID); err != nil {
		t.Fatalf("increment membership version: %v", err)
	}
	_, _, err = resolver.ResolveAuthenticatedOrganization(
		ctx,
		userID,
		sessionID,
		"",
		"",
	)
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "ORGANIZATION_CONTEXT_STALE" {
		t.Fatalf("stale session error = %v", err)
	}
}
