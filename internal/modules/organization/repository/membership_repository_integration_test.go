//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestMembershipRepositoryLifecycleAndOwnerGuardIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	organizationRepo := NewOrganizationRepository(db)
	membershipRepo := NewMembershipRepository(db)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("membership"), ".", "-")

	organization, err := organizationRepo.Create(ctx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "org-" + suffix,
		Name:          "Membership Repository Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	userIDs := make([]string, 3)
	for index := range userIDs {
		email := fmt.Sprintf("membership-%d-%s@example.test", index, suffix)
		if err := db.QueryRow(ctx, `
			INSERT INTO users (name, email, status)
			VALUES ($1, $2, 'active')
			RETURNING id
		`, fmt.Sprintf("Membership User %d", index+1), email).Scan(&userIDs[index]); err != nil {
			t.Fatalf("create user %d: %v", index+1, err)
		}
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_memberships WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, userIDs)
	})

	now := time.Now().UTC()
	owner, err := membershipRepo.Create(ctx, CreateMembershipParams{
		OrganizationID: organization.ID,
		UserID:         userIDs[0],
		Status:         model.MembershipStatusActive,
		IsOwner:        true,
		AcceptedAt:     &now,
	})
	if err != nil {
		t.Fatalf("create owner membership: %v", err)
	}
	member, err := membershipRepo.Create(ctx, CreateMembershipParams{
		OrganizationID: organization.ID,
		UserID:         userIDs[1],
		Status:         model.MembershipStatusInvited,
		InvitedEmail:   fmt.Sprintf("membership-1-%s@example.test", suffix),
		InvitedAt:      &now,
	})
	if err != nil {
		t.Fatalf("create invited membership: %v", err)
	}

	activeMember, err := membershipRepo.UpdateStatus(
		ctx,
		member.ID,
		model.MembershipStatusActive,
		now.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("activate membership: %v", err)
	}
	if activeMember.Version <= member.Version {
		t.Fatalf("membership version = %d, want greater than %d", activeMember.Version, member.Version)
	}
	if _, err := membershipRepo.FindActiveVersion(
		ctx,
		activeMember.UserID,
		organization.ID,
		member.Version,
	); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("FindActiveVersion() stale error = %v, want pgx.ErrNoRows", err)
	}
	if _, err := membershipRepo.FindActiveVersion(
		ctx,
		activeMember.UserID,
		organization.ID,
		activeMember.Version,
	); err != nil {
		t.Fatalf("FindActiveVersion() current error = %v", err)
	}

	organizationMembers, total, err := membershipRepo.ListByOrganization(
		ctx,
		organization.ID,
		MembershipListFilter{Status: model.MembershipStatusActive},
	)
	if err != nil {
		t.Fatalf("ListByOrganization() error = %v", err)
	}
	if total != 2 || len(organizationMembers) != 2 {
		t.Fatalf("ListByOrganization() total = %d, length = %d, want 2", total, len(organizationMembers))
	}

	userOrganizations, total, err := membershipRepo.ListByUser(
		ctx,
		activeMember.UserID,
		MembershipListFilter{Status: model.MembershipStatusActive},
	)
	if err != nil {
		t.Fatalf("ListByUser() error = %v", err)
	}
	if total != 1 || len(userOrganizations) != 1 ||
		userOrganizations[0].Organization.ID != organization.ID {
		t.Fatalf("ListByUser() total = %d, results = %#v", total, userOrganizations)
	}

	if err := membershipRepo.TransferOwnership(
		ctx,
		organization.ID,
		owner.ID,
		activeMember.ID,
		now.Add(2*time.Second),
	); err != nil {
		t.Fatalf("TransferOwnership() error = %v", err)
	}
	owner, err = membershipRepo.FindActive(ctx, owner.UserID, organization.ID)
	if err != nil {
		t.Fatalf("FindActive() former owner error = %v", err)
	}
	activeMember, err = membershipRepo.FindActive(ctx, activeMember.UserID, organization.ID)
	if err != nil {
		t.Fatalf("FindActive() new owner error = %v", err)
	}
	if owner.IsOwner || !activeMember.IsOwner {
		t.Fatalf("ownership transfer result old=%v new=%v", owner.IsOwner, activeMember.IsOwner)
	}

	secondOwner, err := membershipRepo.Create(ctx, CreateMembershipParams{
		OrganizationID: organization.ID,
		UserID:         userIDs[2],
		Status:         model.MembershipStatusActive,
		IsOwner:        true,
		AcceptedAt:     &now,
	})
	if err != nil {
		t.Fatalf("create second owner membership: %v", err)
	}

	ownerIDs := []string{activeMember.ID, secondOwner.ID}
	errs := make(chan error, len(ownerIDs))
	var waitGroup sync.WaitGroup
	for _, membershipID := range ownerIDs {
		waitGroup.Add(1)
		go func(id string) {
			defer waitGroup.Done()
			_, updateErr := membershipRepo.UpdateStatus(
				context.Background(),
				id,
				model.MembershipStatusRemoved,
				time.Now().UTC(),
			)
			errs <- updateErr
		}(membershipID)
	}
	waitGroup.Wait()
	close(errs)

	var succeeded int
	var rejected int
	for updateErr := range errs {
		switch {
		case updateErr == nil:
			succeeded++
		case errors.Is(updateErr, ErrLastActiveOwner):
			rejected++
		default:
			t.Fatalf("concurrent owner removal error = %v", updateErr)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent owner removal succeeded=%d rejected=%d, want 1 each", succeeded, rejected)
	}

	var activeOwnerCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM organization_memberships
		WHERE organization_id = $1
			AND is_owner = true
			AND status = 'active'
			AND removed_at IS NULL
	`, organization.ID).Scan(&activeOwnerCount); err != nil {
		t.Fatalf("count active owners: %v", err)
	}
	if activeOwnerCount != 1 {
		t.Fatalf("active owner count = %d, want 1", activeOwnerCount)
	}
}
