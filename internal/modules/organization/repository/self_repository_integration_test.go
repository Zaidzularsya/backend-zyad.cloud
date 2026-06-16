//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestSelfRepositoryUpdateWritesTenantAuditIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("self-repository"), ".", "-")

	organization, err := NewOrganizationRepository(db).Create(
		ctx,
		CreateOrganizationParams{
			Type:          coretenant.OrganizationTypeCustomer,
			Slug:          "org-" + suffix,
			Name:          "Self Repository Test",
			Status:        coretenant.OrganizationStatusActive,
			DataPlacement: coretenant.DataPlacementShared,
		},
	)
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status)
		VALUES ('Self Repository Owner', $1, 'active')
		RETURNING id
	`, fmt.Sprintf("self-repository-%s@example.test", suffix)).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now().UTC()
	membership, err := NewMembershipRepository(db).Create(
		ctx,
		CreateMembershipParams{
			OrganizationID: organization.ID,
			UserID:         userID,
			Status:         model.MembershipStatusActive,
			IsOwner:        true,
			AcceptedAt:     &now,
		},
	)
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}
	var roleID string
	if err := db.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug, description)
		VALUES ($1, $2, 'Self repository integration role')
		RETURNING id
	`, "Self Repository "+suffix, "self_repository_"+suffix).Scan(&roleID); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_roles (
			id, user_id, role_id, organization_id, assigned_by, assigned_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $1, $4)
	`, userID, roleID, organization.ID, now); err != nil {
		t.Fatalf("assign organization role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM user_roles WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_memberships WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM roles WHERE id = $1`, roleID)
	})

	details, total, err := NewMembershipServiceRepository(db).
		ListDetailsByOrganization(ctx, organization.ID, MembershipListFilter{})
	if err != nil {
		t.Fatalf("ListDetailsByOrganization() error = %v", err)
	}
	if total != 1 || len(details) != 1 ||
		details[0].UserEmail == "" ||
		len(details[0].RoleIDs) != 1 ||
		details[0].RoleIDs[0] != roleID {
		t.Fatalf(
			"ListDetailsByOrganization() details=%#v total=%d",
			details,
			total,
		)
	}

	name := "Updated Self Repository Test"
	timezone := "Asia/Makassar"
	updated, err := NewSelfRepository(db).Update(
		ctx,
		organization.ID,
		UpdateCurrentOrganizationParams{
			Name:         &name,
			Timezone:     &timezone,
			ActorUserID:  userID,
			MembershipID: membership.ID,
			ChangedAt:    now.Add(time.Minute),
		},
	)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != name || updated.Timezone != timezone {
		t.Fatalf("Update() organization = %#v", updated)
	}

	var auditMembershipID string
	var auditActorUserID string
	if err := db.QueryRow(ctx, `
		SELECT membership_id, actor_user_id
		FROM audit_logs
		WHERE organization_id = $1
			AND event = 'organization_profile_updated'
		ORDER BY created_at DESC
		LIMIT 1
	`, organization.ID).Scan(&auditMembershipID, &auditActorUserID); err != nil {
		t.Fatalf("read organization update audit: %v", err)
	}
	if auditMembershipID != membership.ID || auditActorUserID != userID {
		t.Fatalf(
			"audit membership_id=%q actor_user_id=%q",
			auditMembershipID,
			auditActorUserID,
		)
	}
}
