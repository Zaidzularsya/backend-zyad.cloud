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
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestMembershipServiceLifecycleIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	organizationRepo := repository.NewOrganizationRepository(db)
	membershipRepo := repository.NewMembershipRepository(db)
	serviceRepo := repository.NewMembershipServiceRepository(db)
	membershipService := NewMembershipService(serviceRepo)
	suffix := strings.ReplaceAll(testutil.UniqueCode("membership-service"), ".", "-")

	organization, err := organizationRepo.Create(ctx, repository.CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "org-" + suffix,
		Name:          "Membership Service Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	userIDs := make([]string, 2)
	emails := make([]string, 2)
	for index := range userIDs {
		emails[index] = fmt.Sprintf("membership-service-%d-%s@example.test", index, suffix)
		if err := db.QueryRow(ctx, `
			INSERT INTO users (name, email, status)
			VALUES ($1, $2, 'active')
			RETURNING id
		`, fmt.Sprintf("Membership Service User %d", index+1), emails[index]).
			Scan(&userIDs[index]); err != nil {
			t.Fatalf("create user %d: %v", index+1, err)
		}
	}
	var roleID string
	if err := db.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug, description)
		VALUES ($1, $2, 'membership service integration role')
		RETURNING id
	`, "Membership Service Role "+suffix, "membership-service-role-"+suffix).
		Scan(&roleID); err != nil {
		t.Fatalf("create role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM refresh_tokens WHERE user_id = ANY($1::uuid[])`, userIDs)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM sessions WHERE user_id = ANY($1::uuid[])`, userIDs)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM user_roles WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_memberships WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM roles WHERE id = $1`, roleID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, userIDs)
	})

	now := time.Now().UTC()
	owner, err := membershipRepo.Create(ctx, repository.CreateMembershipParams{
		OrganizationID: organization.ID,
		UserID:         userIDs[0],
		Status:         model.MembershipStatusActive,
		IsOwner:        true,
		AcceptedAt:     &now,
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	invite, err := membershipService.Invite(ctx, InviteMemberInput{
		OrganizationID: organization.ID,
		Email:          emails[1],
		RoleIDs:        []string{roleID},
		ActorUserID:    userIDs[0],
	})
	if err != nil {
		t.Fatalf("invite member: %v", err)
	}
	if invite.Membership.Status != model.MembershipStatusInvited ||
		invite.Membership.InvitationTokenHash == invite.Token {
		t.Fatalf("invite result = %#v", invite)
	}

	member, err := membershipService.Accept(ctx, AcceptInvitationInput{
		UserID: userIDs[1],
		Token:  invite.Token,
	})
	if err != nil {
		t.Fatalf("accept invitation: %v", err)
	}
	if !member.IsActive() {
		t.Fatalf("accepted membership = %#v", member)
	}

	sessionID := createMembershipSession(t, db, member, "role-change-"+suffix)
	if _, err := membershipService.SyncRoles(ctx, SyncMembershipRolesInput{
		OrganizationID: organization.ID,
		MembershipID:   member.ID,
		RoleIDs:        nil,
		ActorUserID:    userIDs[0],
	}); err != nil {
		t.Fatalf("sync roles: %v", err)
	}
	assertSessionRevoked(t, db, sessionID)

	sessionID = createMembershipSession(t, db, member, "suspend-"+suffix)
	suspended, err := membershipService.ChangeStatus(ctx, ChangeMembershipStatusInput{
		OrganizationID: organization.ID,
		MembershipID:   member.ID,
		Status:         model.MembershipStatusSuspended,
		Reason:         "security review",
		ActorUserID:    userIDs[0],
	})
	if err != nil {
		t.Fatalf("suspend member: %v", err)
	}
	if suspended.Status != model.MembershipStatusSuspended {
		t.Fatalf("suspended membership = %#v", suspended)
	}
	assertSessionRevoked(t, db, sessionID)

	member, err = membershipService.ChangeStatus(ctx, ChangeMembershipStatusInput{
		OrganizationID: organization.ID,
		MembershipID:   member.ID,
		Status:         model.MembershipStatusActive,
		ActorUserID:    userIDs[0],
	})
	if err != nil {
		t.Fatalf("restore member: %v", err)
	}
	if err := membershipService.TransferOwnership(ctx, TransferOwnershipInput{
		OrganizationID:   organization.ID,
		FromMembershipID: owner.ID,
		ToMembershipID:   member.ID,
		ActorUserID:      userIDs[0],
	}); err != nil {
		t.Fatalf("transfer ownership: %v", err)
	}
	if _, err := membershipService.SyncRoles(ctx, SyncMembershipRolesInput{
		OrganizationID: organization.ID,
		MembershipID:   owner.ID,
		RoleIDs:        []string{roleID},
		ActorUserID:    userIDs[0],
	}); err != nil {
		t.Fatalf("assign old owner role: %v", err)
	}
	if _, err := membershipService.ChangeStatus(ctx, ChangeMembershipStatusInput{
		OrganizationID: organization.ID,
		MembershipID:   owner.ID,
		Status:         model.MembershipStatusRemoved,
		Reason:         "owner transfer complete",
		ActorUserID:    userIDs[0],
	}); err != nil {
		t.Fatalf("remove old owner: %v", err)
	}
	var oldOwnerRoleCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM user_roles
		WHERE user_id = $1
			AND organization_id = $2
	`, owner.UserID, organization.ID).Scan(&oldOwnerRoleCount); err != nil {
		t.Fatalf("read old owner roles: %v", err)
	}
	if oldOwnerRoleCount != 0 {
		t.Fatalf("removed membership retained %d organization roles", oldOwnerRoleCount)
	}

	_, err = membershipService.ChangeStatus(ctx, ChangeMembershipStatusInput{
		OrganizationID: organization.ID,
		MembershipID:   member.ID,
		Status:         model.MembershipStatusRemoved,
		Reason:         "attempt to remove last owner",
		ActorUserID:    userIDs[0],
	})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "MEMBERSHIP_LAST_OWNER_REQUIRED" {
		t.Fatalf("remove last owner error = %v", err)
	}
}

func createMembershipSession(
	t *testing.T,
	db *database.Pool,
	membership model.Membership,
	suffix string,
) string {
	t.Helper()
	var sessionID string
	err := db.QueryRow(context.Background(), `
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
	`, membership.UserID, "session-"+suffix, membership.OrganizationID,
		membership.ID, membership.Version).Scan(&sessionID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO refresh_tokens (session_id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, now() + interval '1 day')
	`, sessionID, membership.UserID, "refresh-"+suffix); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	return sessionID
}

func assertSessionRevoked(
	t *testing.T,
	db *database.Pool,
	sessionID string,
) {
	t.Helper()
	var sessionRevoked bool
	var refreshRevoked bool
	err := db.QueryRow(context.Background(), `
		SELECT s.revoked_at IS NOT NULL, rt.revoked_at IS NOT NULL
		FROM sessions s
		JOIN refresh_tokens rt ON rt.session_id = s.id
		WHERE s.id = $1
	`, sessionID).Scan(&sessionRevoked, &refreshRevoked)
	if err != nil {
		t.Fatalf("read session revocation: %v", err)
	}
	if !sessionRevoked || !refreshRevoked {
		t.Fatalf("revocation state = session:%v refresh:%v", sessionRevoked, refreshRevoked)
	}
}
