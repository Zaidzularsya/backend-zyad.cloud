//go:build integration

package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

// Written for Task 14; not executed (no test DB in that task).
func TestActivationSettleByInvoiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	pages := repository.NewPageRepository(db)
	acts := repository.NewActivationRepository(db)

	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', $3, 'Organization A', 'active'),
		       ($2, 'customer', $4, 'Organization B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID, tenants.B.OrganizationID, testutil.UniqueCode("set-a"), testutil.UniqueCode("set-b")); err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	page, err := pages.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name: "Undangan", Title: "Undangan", Slug: strings.ReplaceAll("set-"+testutil.UniqueCode("p"), ".", "-"),
		Type: domain.PageTypeCampaign, Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic, Locale: "id-ID", Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), "DELETE FROM landing_pages WHERE id = $1", page.ID) })

	_, hash, _ := domain.NewActivationPreviewToken()
	act, err := acts.Create(ctx, tenants.A.Scope, repository.CreateActivationParams{
		PageID: page.ID, BuyerName: "Budi", BuyerContact: "628123", Price: "150000.00",
		DurationValue: 7, DurationUnit: domain.ActivationDurationDay, PreviewTokenHash: hash,
		InvoiceDueAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create activation: %v", err)
	}
	var accountID, invoiceID string
	if err := db.QueryRow(ctx, `INSERT INTO receivable_accounts (organization_id, name) VALUES ($1, 'Budi') RETURNING id`,
		tenants.A.OrganizationID).Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO receivable_invoices (organization_id, account_id) VALUES ($1, $2) RETURNING id`,
		tenants.A.OrganizationID, accountID).Scan(&invoiceID); err != nil {
		t.Fatalf("insert invoice: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), "DELETE FROM landing_page_activations WHERE page_id = $1", page.ID)
		_, _ = db.Exec(context.Background(), "DELETE FROM receivable_invoices WHERE id = $1", invoiceID)
		_, _ = db.Exec(context.Background(), "DELETE FROM receivable_accounts WHERE id = $1", accountID)
	})
	if _, err := acts.SetInvoice(ctx, tenants.A.Scope, act.ID, invoiceID); err != nil {
		t.Fatalf("set invoice: %v", err)
	}

	// Another organization's scope must not see it.
	if res, err := acts.SettleByInvoice(ctx, tenants.B.Scope, invoiceID, time.Now()); err != nil || res.Found {
		t.Fatalf("foreign scope: found=%v err=%v", res.Found, err)
	}

	paidAt := time.Now().UTC().Truncate(time.Microsecond)
	res, err := acts.SettleByInvoice(ctx, tenants.A.Scope, invoiceID, paidAt)
	if err != nil || !res.Applied || !res.Published {
		t.Fatalf("settle: %+v err=%v", res, err)
	}
	got, err := pages.FindByID(ctx, tenants.A.Scope, page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PageStatusPublished || got.ActiveUntil == nil || !got.ActiveUntil.Equal(paidAt.AddDate(0, 0, 7)) {
		t.Fatalf("page = %s until %v", got.Status, got.ActiveUntil)
	}
	if again, err := acts.SettleByInvoice(ctx, tenants.A.Scope, invoiceID, paidAt.Add(time.Hour)); err != nil || again.Applied {
		t.Fatalf("repeat: applied=%v err=%v", again.Applied, err)
	}
}
