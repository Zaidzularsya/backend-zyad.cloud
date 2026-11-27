//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestSocialIdentitySchema(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := strings.ReplaceAll(testutil.UniqueCode("social"), ".", "_")
	email := fmt.Sprintf("budi-%s@x.test", suffix)

	var orgID, otherOrgID, userID, otherUserID string
	if err := db.QueryRow(ctx, `
		INSERT INTO organizations (type, slug, name, status, data_placement)
		VALUES ('customer', $1, $1, 'active', 'shared')
		RETURNING id
	`, "social-"+suffix).Scan(&orgID); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO organizations (type, slug, name, status, data_placement)
		VALUES ('customer', $1, $1, 'active', 'shared')
		RETURNING id
	`, "social-other-"+suffix).Scan(&otherOrgID); err != nil {
		t.Fatalf("insert other organization: %v", err)
	}
	t.Cleanup(func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		_, _ = db.Exec(c, `DELETE FROM organizations WHERE id = ANY($1::uuid[])`, []string{orgID, otherOrgID})
		_, _ = db.Exec(c, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{userID, otherUserID})
	})
	for i, dst := range []*string{&userID, &otherUserID} {
		if err := db.QueryRow(ctx, `
			INSERT INTO users (name, email, status)
			VALUES ($1, $2, 'active')
			RETURNING id
		`, "Social Test", fmt.Sprintf("user%d-%s@x.test", i, suffix)).Scan(dst); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	// crm_contacts is under FORCE RLS: seed rows inside the organization context.
	contactID := insertContact(t, ctx, db, orgID, "Budi", false, "  "+strings.ToUpper(email)+" ")
	otherContactID := insertContact(t, ctx, db, orgID, "Siti", false, fmt.Sprintf("siti-%s@x.test", suffix))
	_ = insertContact(t, ctx, db, orgID, "Budi Lama", true, email)
	t.Run("live customer link is unique per contact and per user/org", func(t *testing.T) {
		insertLink := func(user, contact, status string) error {
			_, err := db.Exec(ctx, `
				INSERT INTO customer_links (user_id, organization_id, crm_contact_id, status, verified_via)
				VALUES ($1, $2, $3, $4, 'email')
			`, user, orgID, contact, status)
			return err
		}
		if err := insertLink(userID, contactID, "active"); err != nil {
			t.Fatalf("first link: %v", err)
		}
		if err := insertLink(otherUserID, contactID, "active"); err == nil {
			t.Fatal("second live link for same contact must violate unique index")
		}
		if err := insertLink(userID, otherContactID, "pending_user"); err == nil {
			t.Fatal("second live link for same user/org must violate unique index")
		}
		if err := insertLink(otherUserID, contactID, "revoked"); err != nil {
			t.Fatalf("revoked link must not count as live: %v", err)
		}
		if err := insertLink(userID, otherContactID, "bogus"); err == nil {
			t.Fatal("invalid status must violate check constraint")
		}
	})

	t.Run("referral columns", func(t *testing.T) {
		if _, err := db.Exec(ctx, `UPDATE users SET referral_code = 'ABCD2345' WHERE id = $1`, userID); err != nil {
			t.Fatalf("set referral code: %v", err)
		}
		if _, err := db.Exec(ctx, `UPDATE users SET referral_code = 'ABCD2345' WHERE id = $1`, otherUserID); err == nil {
			t.Fatal("duplicate referral_code must be rejected")
		}
		if _, err := db.Exec(ctx, `UPDATE users SET referred_by_user_id = id WHERE id = $1`, userID); err == nil {
			t.Fatal("self referral must be rejected")
		}
	})

	t.Run("empty lookup context never exposes blank-email contacts", func(t *testing.T) {
		blankID := insertContact(t, ctx, db, orgID, "Kosong", false, "")
		spaceID := insertContact(t, ctx, db, orgID, "Spasi", false, "   ")
		for _, orgCtx := range []string{"", otherOrgID} {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, orgCtx); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `SELECT set_config('app.crm_contact_email_lookup', '', true)`); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM crm_contacts WHERE id = ANY($1::uuid[])`, []string{blankID, spaceID}).Scan(&n); err != nil {
				t.Fatal(err)
			}
			tx.Rollback(context.Background())
			if n != 0 {
				t.Fatalf("org context %q with empty lookup sees %d blank-email contacts, want 0", orgCtx, n)
			}
		}
	})

	t.Run("email lookup context exposes only matching live contacts", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		// No app.organization_id: only the narrow lookup context is set.
		if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', '', true)`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.crm_contact_email_lookup', $1, true)`, strings.ToLower(email)); err != nil {
			t.Fatal(err)
		}
		rows, err := tx.Query(ctx, `SELECT id::text FROM crm_contacts WHERE organization_id = $1`, orgID)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			got = append(got, id)
		}
		rows.Close()
		if len(got) != 1 || got[0] != contactID {
			t.Fatalf("lookup visible = %v, want only %s (soft-deleted and other emails hidden)", got, contactID)
		}

		tag, err := tx.Exec(ctx, `UPDATE crm_contacts SET first_name = 'Hacked' WHERE id = $1`, contactID)
		if err != nil {
			t.Fatalf("update in lookup mode: %v", err)
		}
		if tag.RowsAffected() != 0 {
			t.Fatalf("update in lookup mode affected %d rows, want 0", tag.RowsAffected())
		}
	})
}

// insertContact inserts a CRM contact (optionally soft-deleted) inside the organization RLS context.
func insertContact(t *testing.T, ctx context.Context, db *database.Pool, orgID, firstName string, deleted bool, email string) string {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, orgID); err != nil {
		t.Fatal(err)
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO crm_contacts (organization_id, first_name, email, deleted_at)
		VALUES ($1, $2, $3, CASE WHEN $4::boolean THEN now() END)
		RETURNING id::text
	`, orgID, firstName, email, deleted).Scan(&id)
	if err != nil {
		t.Fatalf("insert contact: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}
