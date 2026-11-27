//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"zyad.cloud/internal/modules/social/domain"
	"zyad.cloud/internal/platform/database/testutil"
)

// Two users claim the same CRM contact at once: the partial unique index lets
// exactly one win; the other gets ErrCustomerLinkExists. Each activation is audited.
func TestConcurrentAcceptOnlyOneWins(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repo := NewCustomerLinkRepository(db)

	suffix := testutil.UniqueCode("claim")
	var orgID string
	if err := db.QueryRow(ctx, `
		INSERT INTO organizations (type, slug, name, status, data_placement)
		VALUES ('customer', $1, 'Claim Test ISP', 'active', 'shared') RETURNING id::text`,
		fmt.Sprintf("claim-%d", time.Now().UnixNano())).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	var userA, userB string
	for i, dst := range []*string{&userA, &userB} {
		if err := db.QueryRow(ctx, `
			INSERT INTO users (name, email, status) VALUES ('Claim Test', $1, 'active') RETURNING id::text`,
			fmt.Sprintf("claim%d-%s@x.test", i, suffix)).Scan(dst); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	var contactID, otherContactID string
	if err := db.QueryRow(ctx, `SELECT gen_random_uuid()::text, gen_random_uuid()::text`).Scan(&contactID, &otherContactID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		_, _ = db.Exec(c, `DELETE FROM audit_logs WHERE organization_id = $1::uuid`, orgID)
		_, _ = db.Exec(c, `DELETE FROM customer_links WHERE organization_id = $1::uuid`, orgID)
		_, _ = db.Exec(c, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{userA, userB})
		_, _ = db.Exec(c, `DELETE FROM organizations WHERE id = $1::uuid`, orgID)
	})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, userID := range []string{userA, userB} {
		wg.Add(1)
		go func(i int, userID string) {
			defer wg.Done()
			_, errs[i] = repo.CreateActive(ctx, domain.CustomerLink{
				UserID: userID, OrganizationID: orgID, ContactID: contactID, VerifiedVia: domain.VerifiedViaEmail,
			})
		}(i, userID)
	}
	wg.Wait()

	wins, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, domain.ErrCustomerLinkExists):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d, want 1/1", wins, conflicts)
	}
	winner := userA
	if errs[0] != nil {
		winner = userB
	}

	var audits int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE module = 'social' AND event = 'customer_link_activated' AND organization_id = $1::uuid`, orgID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("activation audits = %d, want 1", audits)
	}

	// The winner cannot hold a second live link in the same organization.
	if _, err := repo.CreateActive(ctx, domain.CustomerLink{
		UserID: winner, OrganizationID: orgID, ContactID: otherContactID, VerifiedVia: domain.VerifiedViaEmail,
	}); !errors.Is(err, domain.ErrCustomerLinkExists) {
		t.Fatalf("second link same org err = %v, want ErrCustomerLinkExists", err)
	}

	// After a revoke the contact is free again; a second revoke reports not found.
	links, err := repo.ListByUser(ctx, winner)
	if err != nil || len(links) != 1 {
		t.Fatalf("ListByUser = %+v, %v", links, err)
	}
	if err := repo.Revoke(ctx, links[0], winner, false); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := repo.Revoke(ctx, links[0], winner, false); !errors.Is(err, domain.ErrCustomerLinkNotFound) {
		t.Fatalf("second Revoke err = %v, want ErrCustomerLinkNotFound", err)
	}
	if live, err := repo.HasLiveLinkForContact(ctx, orgID, contactID); err != nil || live {
		t.Fatalf("HasLiveLinkForContact after revoke = %v, %v", live, err)
	}
	loser := userB
	if winner == userB {
		loser = userA
	}
	if err := repo.InsertDeclined(ctx, loser, orgID, otherContactID); err != nil {
		t.Fatalf("InsertDeclined: %v", err)
	}
	if _, err := repo.CreateActive(ctx, domain.CustomerLink{
		UserID: loser, OrganizationID: orgID, ContactID: contactID, VerifiedVia: domain.VerifiedViaEmail,
	}); err != nil {
		t.Fatalf("claim after revoke: %v", err)
	}
}

func TestSettingsRepositoryUpsert(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repo := NewSettingsRepository(db)

	var orgID string
	if err := db.QueryRow(ctx, `
		INSERT INTO organizations (type, slug, name, status, data_placement)
		VALUES ('customer', $1, 'Settings Test', 'active', 'shared') RETURNING id::text`,
		fmt.Sprintf("socset-%d", time.Now().UnixNano())).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1::uuid`, orgID)
	})

	if _, found, err := repo.Get(ctx, orgID); err != nil || found {
		t.Fatalf("Get before upsert found=%v err=%v", found, err)
	}
	for _, want := range []bool{false, true} {
		saved, err := repo.Upsert(ctx, domain.TenantSettings{OrganizationID: orgID, CustomerClaimAutoOffer: want})
		if err != nil || saved.CustomerClaimAutoOffer != want || saved.UpdatedAt == nil {
			t.Fatalf("Upsert(%v) = %+v, %v", want, saved, err)
		}
		got, found, err := repo.Get(ctx, orgID)
		if err != nil || !found || got.CustomerClaimAutoOffer != want {
			t.Fatalf("Get after Upsert(%v) = %+v, %v, %v", want, got, found, err)
		}
	}
}
