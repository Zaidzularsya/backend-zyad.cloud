//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	coreauth "zyad.cloud/internal/core/auth"
	"zyad.cloud/internal/modules/social/domain"
	"zyad.cloud/internal/platform/database/testutil"
)

// A resend supersedes the open invite; accepting consumes the token exactly once
// (also under concurrency), links with verified_via=invite, and an expired invite
// cannot be accepted.
func TestInviteRepositoryLifecycle(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repo := NewInviteRepository(db)

	suffix := testutil.UniqueCode("invite")
	var orgID string
	if err := db.QueryRow(ctx, `
		INSERT INTO organizations (type, slug, name, status, data_placement)
		VALUES ('customer', $1, 'Invite Test ISP', 'active', 'shared') RETURNING id::text`,
		fmt.Sprintf("invite-%d", time.Now().UnixNano())).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	var userA, userB string
	for i, dst := range []*string{&userA, &userB} {
		if err := db.QueryRow(ctx, `
			INSERT INTO users (name, email, status) VALUES ('Invite Test', $1, 'active') RETURNING id::text`,
			fmt.Sprintf("invite%d-%s@x.test", i, suffix)).Scan(dst); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	var contactID, expiredContactID string
	if err := db.QueryRow(ctx, `SELECT gen_random_uuid()::text, gen_random_uuid()::text`).Scan(&contactID, &expiredContactID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		_, _ = db.Exec(c, `DELETE FROM audit_logs WHERE organization_id = $1::uuid`, orgID)
		_, _ = db.Exec(c, `DELETE FROM customer_links WHERE organization_id = $1::uuid`, orgID)
		_, _ = db.Exec(c, `DELETE FROM customer_claim_invites WHERE organization_id = $1::uuid`, orgID)
		_, _ = db.Exec(c, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{userA, userB})
		_, _ = db.Exec(c, `DELETE FROM organizations WHERE id = $1::uuid`, orgID)
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	newInvite := func(contact, token string, created time.Time) domain.ClaimInvite {
		inv, err := repo.Create(ctx, domain.ClaimInvite{
			OrganizationID: orgID, ContactID: contact, Email: "budi-" + suffix + "@x.test",
			TokenHash: coreauth.HashToken(token), ExpiresAt: created.Add(domain.InviteTTL),
			CreatedBy: userA, CreatedAt: created,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return inv
	}

	first := newInvite(contactID, "first-"+suffix, now)
	newInvite(contactID, "second-"+suffix, now.Add(time.Second))
	got, found, err := repo.FindByTokenHash(ctx, first.TokenHash)
	if err != nil || !found || got.UsedAt == nil {
		t.Fatalf("first invite must be superseded: found=%v used=%v err=%v", found, got.UsedAt, err)
	}
	if _, err := repo.Accept(ctx, first.TokenHash, userA, now.Add(time.Minute)); !errors.Is(err, domain.ErrInviteInvalid) {
		t.Fatalf("accept superseded: err = %v, want ErrInviteInvalid", err)
	}

	secondHash := coreauth.HashToken("second-" + suffix)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, userID := range []string{userA, userB} {
		wg.Add(1)
		go func(i int, userID string) {
			defer wg.Done()
			_, errs[i] = repo.Accept(ctx, secondHash, userID, now.Add(time.Minute))
		}(i, userID)
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, domain.ErrInviteInvalid):
		default:
			t.Fatalf("unexpected accept error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d, want exactly 1 (%v)", wins, errs)
	}
	var via string
	if err := db.QueryRow(ctx, `
		SELECT verified_via FROM customer_links
		WHERE organization_id = $1::uuid AND crm_contact_id = $2::uuid AND status = 'active'`,
		orgID, contactID).Scan(&via); err != nil || via != domain.VerifiedViaInvite {
		t.Fatalf("link verified_via = %q, err %v", via, err)
	}

	expired := newInvite(expiredContactID, "expired-"+suffix, now.Add(-domain.InviteTTL-time.Hour))
	if _, err := repo.Accept(ctx, expired.TokenHash, userA, now); !errors.Is(err, domain.ErrInviteInvalid) {
		t.Fatalf("accept expired: err = %v, want ErrInviteInvalid", err)
	}
}
