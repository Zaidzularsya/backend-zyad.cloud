//go:build integration

package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

// ListByOrganization / FindByID / RotatePreview under FORCE RLS: one organization must never see or
// change another's activations, and only an OPEN activation can have its preview link rotated. Only
// meaningful with a non-superuser test connection (a superuser bypasses RLS).
func TestActivationManageIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	var bypass bool
	if err := db.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&bypass); err != nil {
		t.Fatal(err)
	}
	if bypass {
		t.Skip("koneksi uji superuser/BYPASSRLS: RLS tidak berlaku, test tidak bermakna")
	}

	tenants := testutil.NewTenantPair(t)
	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', $3, 'Organization A', 'active'),
		       ($2, 'customer', $4, 'Organization B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID, tenants.B.OrganizationID, orgSlug("mg-a"), orgSlug("mg-b")); err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	pages := repository.NewPageRepository(db)
	acts := repository.NewActivationRepository(db)

	newPage := func(scope coretenant.Scope, title string) domain.LandingPage {
		p, err := pages.Create(ctx, scope, repository.CreatePageParams{
			Name: title, Title: title, Slug: strings.ReplaceAll("mg-"+testutil.UniqueCode("p"), ".", "-"),
			Type: domain.PageTypeCampaign, Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusDraft,
			Visibility: domain.PageVisibilityPublic, Locale: "id-ID", Timezone: "Asia/Jakarta",
		})
		if err != nil {
			t.Fatalf("create page: %v", err)
		}
		t.Cleanup(func() { _, _ = db.Exec(context.Background(), "DELETE FROM landing_pages WHERE id = $1", p.ID) })
		return p
	}
	newAct := func(scope coretenant.Scope, pageID, buyer string, due time.Duration) (domain.LandingPageActivation, string) {
		_, hash, _ := domain.NewActivationPreviewToken()
		a, err := acts.Create(ctx, scope, repository.CreateActivationParams{
			PageID: pageID, BuyerName: buyer, BuyerContact: "628123", Price: "150000.00",
			DurationValue: 1, DurationUnit: domain.ActivationDurationMonth, PreviewTokenHash: hash,
			InvoiceDueAt: time.Now().UTC().Add(due),
		})
		if err != nil {
			t.Fatalf("create activation: %v", err)
		}
		return a, hash
	}

	pageA1, pageA2, pageB := newPage(tenants.A.Scope, "Undangan Satu"), newPage(tenants.A.Scope, "Undangan Dua"), newPage(tenants.B.Scope, "Milik B")
	openA, openHash := newAct(tenants.A.Scope, pageA1.ID, "Budi", time.Hour)
	paidA, _ := newAct(tenants.A.Scope, pageA2.ID, "Sari", time.Hour)
	if _, err := acts.MarkPaid(ctx, tenants.A.Scope, paidA.ID, time.Now().UTC()); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	otherB, _ := newAct(tenants.B.Scope, pageB.ID, "Orang B", time.Hour)

	t.Run("list hanya milik org sendiri, dengan info halaman", func(t *testing.T) {
		items, total, err := acts.ListByOrganization(ctx, tenants.A.Scope, repository.ActivationListFilter{Limit: 20})
		if err != nil || total != 2 || len(items) != 2 {
			t.Fatalf("items=%d total=%d err=%v", len(items), total, err)
		}
		byBuyer := map[string]repository.ActivationListItem{}
		for _, it := range items {
			if it.Activation.OrganizationID != tenants.A.OrganizationID {
				t.Fatalf("bocor org lain: %+v", it.Activation)
			}
			byBuyer[it.Activation.BuyerName] = it
		}
		if byBuyer["Budi"].PageTitle != "Undangan Satu" || byBuyer["Budi"].PageSlug == "" || byBuyer["Budi"].PageStatus != domain.PageStatusDraft {
			t.Fatalf("info halaman = %+v", byBuyer["Budi"])
		}
		if _, leaked := byBuyer["Orang B"]; leaked {
			t.Fatal("aktivasi org B tidak boleh terlihat dari org A")
		}
	})

	t.Run("filter status dan paging", func(t *testing.T) {
		items, total, err := acts.ListByOrganization(ctx, tenants.A.Scope, repository.ActivationListFilter{Status: domain.ActivationStatusPaid, Limit: 20})
		if err != nil || total != 1 || len(items) != 1 || items[0].Activation.ID != paidA.ID {
			t.Fatalf("paid: items=%d total=%d err=%v", len(items), total, err)
		}
		page1, total, err := acts.ListByOrganization(ctx, tenants.A.Scope, repository.ActivationListFilter{Limit: 1, Offset: 0})
		page2, _, err2 := acts.ListByOrganization(ctx, tenants.A.Scope, repository.ActivationListFilter{Limit: 1, Offset: 1})
		if err != nil || err2 != nil || total != 2 || len(page1) != 1 || len(page2) != 1 || page1[0].Activation.ID == page2[0].Activation.ID {
			t.Fatalf("paging: %d/%d total=%d err=%v/%v", len(page1), len(page2), total, err, err2)
		}
	})

	t.Run("FindByID tidak melintasi org", func(t *testing.T) {
		if a, err := acts.FindByID(ctx, tenants.A.Scope, openA.ID); err != nil || a.BuyerName != "Budi" {
			t.Fatalf("own: %+v err=%v", a, err)
		}
		if _, err := acts.FindByID(ctx, tenants.A.Scope, otherB.ID); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("lintas org: err=%v, want ErrNoRows", err)
		}
	})

	t.Run("RotatePreview memutar token dan jendela hanya untuk yang open", func(t *testing.T) {
		_, newHash, _ := domain.NewActivationPreviewToken()
		newDue := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Microsecond)
		got, err := acts.RotatePreview(ctx, tenants.A.Scope, openA.ID, newHash, newDue)
		if err != nil || got.PreviewTokenHash != newHash || !got.InvoiceDueAt.Equal(newDue) {
			t.Fatalf("rotate: %+v err=%v", got, err)
		}
		if _, err := acts.FindByTokenHash(ctx, tenants.A.Scope, openHash); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("token lama harus mati: err=%v", err)
		}
		if a, err := acts.FindByTokenHash(ctx, tenants.A.Scope, newHash); err != nil || a.ID != openA.ID {
			t.Fatalf("token baru harus hidup: %+v err=%v", a, err)
		}
		// sudah dibayar: tidak boleh diputar
		if _, err := acts.RotatePreview(ctx, tenants.A.Scope, paidA.ID, newHash+"x", newDue); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("paid: err=%v, want ErrNoRows", err)
		}
		// org lain: tidak boleh
		if _, err := acts.RotatePreview(ctx, tenants.B.Scope, openA.ID, newHash+"y", newDue); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("lintas org: err=%v, want ErrNoRows", err)
		}
	})
}
