//go:build integration

package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"zyad.cloud/internal/modules/social/domain"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestAttributionRepository(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repo := NewAttributionRepository(db)

	suffix := testutil.UniqueCode("attr")
	var referrerID, subjectID, otherID string
	for i, dst := range []*string{&referrerID, &subjectID, &otherID} {
		if err := db.QueryRow(ctx, `
			INSERT INTO users (name, email, status) VALUES ('Attr Test', $1, 'active') RETURNING id
		`, fmt.Sprintf("attr%d-%s@x.test", i, suffix)).Scan(dst); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	t.Cleanup(func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		_, _ = db.Exec(c, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{referrerID, subjectID, otherID})
	})

	code, err := domain.NewReferralCode(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("assign code once", func(t *testing.T) {
		got, err := repo.AssignReferralCode(ctx, referrerID, code)
		if err != nil || got != code {
			t.Fatalf("AssignReferralCode = %q, %v", got, err)
		}
		// A second assignment keeps the first code.
		again, err := repo.AssignReferralCode(ctx, referrerID, "ZZZZZZZZ")
		if err != nil || again != code {
			t.Fatalf("second AssignReferralCode = %q, %v; want %q", again, err, code)
		}
		if _, err := repo.AssignReferralCode(ctx, otherID, code); !errors.Is(err, domain.ErrReferralCodeTaken) {
			t.Fatalf("colliding code err = %v, want ErrReferralCodeTaken", err)
		}
		owner, found, err := repo.ReferrerByCode(ctx, code)
		if err != nil || !found || owner != referrerID {
			t.Fatalf("ReferrerByCode = %q, %v, %v", owner, found, err)
		}
	})

	t.Run("signup referral written once", func(t *testing.T) {
		ref := domain.SignupReferral{SubjectUserID: subjectID, ReferrerUserID: referrerID, ReferralCode: code, Channel: "wa"}
		set, err := repo.RecordSignupReferral(ctx, ref)
		if err != nil || !set {
			t.Fatalf("first RecordSignupReferral = %v, %v", set, err)
		}
		ref.ReferrerUserID = otherID
		set, err = repo.RecordSignupReferral(ctx, ref)
		if err != nil || set {
			t.Fatalf("second RecordSignupReferral = %v, %v; want false", set, err)
		}
		var referredBy string
		if err := db.QueryRow(ctx, `SELECT referred_by_user_id::text FROM users WHERE id = $1`, subjectID).Scan(&referredBy); err != nil {
			t.Fatal(err)
		}
		if referredBy != referrerID {
			t.Fatalf("referred_by_user_id = %q, want %q", referredBy, referrerID)
		}
	})

	t.Run("visits and counts", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			if err := repo.InsertVisit(ctx, domain.VisitEvent{ReferralCode: code, ReferrerUserID: referrerID, IPHash: "abc"}); err != nil {
				t.Fatalf("InsertVisit: %v", err)
			}
		}
		counts, err := repo.CountEvents(ctx, referrerID)
		if err != nil {
			t.Fatalf("CountEvents: %v", err)
		}
		if counts != (domain.EventCounts{Visits: 2, Signups: 1}) {
			t.Fatalf("counts = %+v", counts)
		}
	})
}
