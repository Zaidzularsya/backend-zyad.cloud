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
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestSwitchServicePersistsSessionAndAuditIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("switch"), ".", "-")
	organizationRepo := repository.NewOrganizationRepository(db)
	membershipRepo := repository.NewMembershipRepository(db)
	switchService := NewSwitchService(repository.NewSwitchRepository(db))

	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status)
		VALUES ('Switch User', $1, 'active')
		RETURNING id
	`, fmt.Sprintf("switch-%s@example.test", suffix)).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	var roleID string
	roleSlug := "switch-role-" + suffix
	if err := db.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug, description)
		VALUES ($1, $2, 'Organization switch integration role')
		RETURNING id
	`, "Switch Role "+suffix, roleSlug).Scan(&roleID); err != nil {
		t.Fatalf("create role: %v", err)
	}
	organizations := make([]model.Organization, 2)
	memberships := make([]model.Membership, 2)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for index := range organizations {
		organization, err := organizationRepo.Create(ctx, repository.CreateOrganizationParams{
			Type:          coretenant.OrganizationTypeCustomer,
			Slug:          fmt.Sprintf("switch-%d-%s", index, suffix),
			Name:          fmt.Sprintf("Switch Organization %d", index+1),
			Status:        coretenant.OrganizationStatusActive,
			DataPlacement: coretenant.DataPlacementShared,
		})
		if err != nil {
			t.Fatalf("create organization %d: %v", index, err)
		}
		organizations[index] = organization
		membership, err := membershipRepo.Create(ctx, repository.CreateMembershipParams{
			OrganizationID: organization.ID,
			UserID:         userID,
			Status:         model.MembershipStatusActive,
			AcceptedAt:     &now,
		})
		if err != nil {
			t.Fatalf("create membership %d: %v", index, err)
		}
		memberships[index] = membership
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id, organization_id)
		VALUES ($1, $2, $3)
	`, userID, roleID, organizations[1].ID); err != nil {
		t.Fatalf("assign organization role: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT version
		FROM organization_memberships
		WHERE id = $1
	`, memberships[1].ID).Scan(&memberships[1].Version); err != nil {
		t.Fatalf("reload membership version: %v", err)
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
	`, userID, "switch-session-"+suffix, organizations[0].ID,
		memberships[0].ID, memberships[0].Version).Scan(&sessionID); err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE actor_user_id = $1`, userID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM sessions WHERE id = $1`, sessionID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM user_roles WHERE user_id = $1`, userID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_memberships WHERE user_id = $1`, userID)
		for _, organization := range organizations {
			_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organization.ID)
		}
		_, _ = db.Exec(cleanupCtx, `DELETE FROM roles WHERE id = $1`, roleID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})

	organizationList, err := switchService.List(ctx, userID, sessionID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(organizationList) != 2 ||
		len(organizationList[1].Membership.RoleIDs) != 1 ||
		organizationList[1].Membership.RoleIDs[0] != roleID ||
		organizationList[1].Membership.RoleSlugs[0] != roleSlug {
		t.Fatalf("List() organization roles = %#v", organizationList)
	}

	response, err := switchService.Switch(
		ctx,
		userID,
		sessionID,
		dto.SwitchOrganizationRequest{OrganizationID: organizations[1].ID},
		SwitchMetadata{
			RequestID: "switch-request",
			IPAddress: "192.0.2.15",
			UserAgent: "integration-test",
		},
	)
	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if response.CurrentOrganization.Organization.ID != organizations[1].ID ||
		!response.CurrentOrganization.IsCurrent ||
		len(response.CurrentOrganization.Membership.RoleIDs) != 1 ||
		response.CurrentOrganization.Membership.RoleIDs[0] != roleID ||
		response.CurrentOrganization.Membership.RoleSlugs[0] != roleSlug {
		t.Fatalf("Switch() response = %#v", response)
	}

	var activeOrganizationID string
	var activeMembershipID string
	var activeMembershipVersion int64
	if err := db.QueryRow(ctx, `
		SELECT
			active_organization_id,
			active_membership_id,
			active_membership_version
		FROM sessions
		WHERE id = $1
	`, sessionID).Scan(
		&activeOrganizationID,
		&activeMembershipID,
		&activeMembershipVersion,
	); err != nil {
		t.Fatalf("read session context: %v", err)
	}
	if activeOrganizationID != organizations[1].ID ||
		activeMembershipID != memberships[1].ID ||
		activeMembershipVersion != memberships[1].Version {
		t.Fatalf(
			"session context = %s/%s/%d",
			activeOrganizationID,
			activeMembershipID,
			activeMembershipVersion,
		)
	}

	var auditCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE event = 'organization_switched'
			AND organization_id = $1
			AND membership_id = $2
			AND session_id = $3
			AND request_id = 'switch-request'
	`, organizations[1].ID, memberships[1].ID, sessionID).Scan(&auditCount); err != nil {
		t.Fatalf("read switch audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("audit count = %d", auditCount)
	}

	_, err = switchService.Switch(
		ctx,
		userID,
		sessionID,
		dto.SwitchOrganizationRequest{
			OrganizationID: "11111111-1111-1111-1111-111111111111",
		},
		SwitchMetadata{},
	)
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "ORGANIZATION_ACCESS_DENIED" {
		t.Fatalf("unauthorized switch error = %v", err)
	}
}
