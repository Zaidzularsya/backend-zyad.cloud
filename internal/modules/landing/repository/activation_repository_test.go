//go:build integration

package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

// Written for Task 12 (page activation); not executed in that task (no test DB).
func TestActivationRepositoryLifecycleAndIsolationIntegration(t *testing.T) {
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
	`, tenants.A.OrganizationID, tenants.B.OrganizationID, orgSlug("act-a"), orgSlug("act-b")); err != nil {
		t.Fatalf("insert organizations: %v", err)
	}

	slug := strings.ReplaceAll("act-"+testutil.UniqueCode("p"), ".", "-")
	page, err := pages.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name: "Undangan", Title: "Undangan", Slug: slug, Type: domain.PageTypeCampaign,
		Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic, Locale: "id-ID", Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), "DELETE FROM landing_pages WHERE id = $1", page.ID) })
	if page.ActiveUntil != nil {
		t.Fatalf("new page active_until = %v, want NULL", page.ActiveUntil)
	}

	raw, hash, err := domain.NewActivationPreviewToken()
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	params := repository.CreateActivationParams{
		PageID: page.ID, BuyerName: "Budi", BuyerContact: "6281234567890", Price: "150000.00",
		DurationValue: 1, DurationUnit: domain.ActivationDurationMonth,
		PreviewTokenHash: hash, InvoiceDueAt: due,
	}
	act, err := acts.Create(ctx, tenants.A.Scope, params)
	if err != nil {
		t.Fatalf("create activation: %v", err)
	}
	if act.Status != domain.ActivationStatusOpen || act.Price != "150000.00" || act.PreviewTokenHash != hash || !act.InvoiceDueAt.Equal(due) {
		t.Fatalf("activation = %+v", act)
	}

	var stored string
	inOrg(t, db, tenants.A.OrganizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT preview_token_hash FROM landing_page_activations WHERE id = $1", act.ID).Scan(&stored)
	})
	if stored == raw || stored != hash {
		t.Fatal("only the token hash may be stored")
	}

	// One open activation per page.
	params2 := params
	_, params2.PreviewTokenHash, _ = domain.NewActivationPreviewToken()
	if _, err := acts.Create(ctx, tenants.A.Scope, params2); !errors.Is(err, repository.ErrActivationOpen) {
		t.Fatalf("a second open activation for the same page: err = %v, want ErrActivationOpen (23505 on %s)", err, "idx_landing_page_activations_one_open_per_page")
	}

	// Another tenant cannot create on, find, or change this page's activation.
	if _, err := acts.Create(ctx, tenants.B.Scope, params2); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign create err = %v, want ErrNoRows", err)
	}
	if _, err := acts.FindByTokenHash(ctx, tenants.B.Scope, hash); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign token lookup err = %v", err)
	}
	if _, err := acts.Void(ctx, tenants.B.Scope, act.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign void err = %v", err)
	}

	if got, err := acts.FindByTokenHash(ctx, tenants.A.Scope, hash); err != nil || got.ID != act.ID {
		t.Fatalf("FindByTokenHash = %+v, %v", got, err)
	}
	if got, err := acts.FindOpenByPage(ctx, tenants.A.Scope, page.ID); err != nil || got.ID != act.ID {
		t.Fatalf("FindOpenByPage = %+v, %v", got, err)
	}
	if m, err := acts.ListOpenByPages(ctx, tenants.A.Scope, []string{page.ID}); err != nil || m[page.ID].ID != act.ID {
		t.Fatalf("ListOpenByPages = %+v, %v", m, err)
	}

	paid, err := acts.MarkPaid(ctx, tenants.A.Scope, act.ID, time.Now().UTC())
	if err != nil || paid.Status != domain.ActivationStatusPaid || paid.PaidAt == nil {
		t.Fatalf("MarkPaid = %+v, %v", paid, err)
	}
	if _, err := acts.MarkPaid(ctx, tenants.A.Scope, act.ID, time.Now().UTC()); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second MarkPaid err = %v, want ErrNoRows", err)
	}
	if _, err := acts.FindOpenByPage(ctx, tenants.A.Scope, page.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("paid activation must not be open, err = %v", err)
	}

	// After the first is closed, a new open activation (renewal) is allowed and can be voided.
	renewal, err := acts.Create(ctx, tenants.A.Scope, params2)
	if err != nil {
		t.Fatalf("renewal create: %v", err)
	}
	if v, err := acts.Void(ctx, tenants.A.Scope, renewal.ID); err != nil || v.Status != domain.ActivationStatusVoid {
		t.Fatalf("Void = %+v, %v", v, err)
	}
}

// Written for Task 13 (activation invoice); not executed in that task (no test DB).
func TestActivationRepositoryReplaceOverdueAndInvoiceLinkIntegration(t *testing.T) {
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
	`, tenants.A.OrganizationID, tenants.B.OrganizationID, orgSlug("rep-a"), orgSlug("rep-b")); err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	slug := strings.ReplaceAll("rep-"+testutil.UniqueCode("p"), ".", "-")
	page, err := pages.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name: "Undangan", Title: "Undangan", Slug: slug, Type: domain.PageTypeCampaign,
		Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic, Locale: "id-ID", Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), "DELETE FROM landing_pages WHERE id = $1", page.ID) })

	now := time.Now().UTC().Truncate(time.Microsecond)
	params := func(due time.Time) repository.CreateActivationParams {
		_, hash, err := domain.NewActivationPreviewToken()
		if err != nil {
			t.Fatal(err)
		}
		return repository.CreateActivationParams{
			PageID: page.ID, BuyerName: "Budi", BuyerContact: "6281234567890", Price: "150000.00",
			DurationValue: 1, DurationUnit: domain.ActivationDurationMonth, PreviewTokenHash: hash, InvoiceDueAt: due,
		}
	}

	first, voided, err := acts.CreateReplacingOverdue(ctx, tenants.A.Scope, params(now.Add(time.Hour)), now)
	if err != nil || voided != nil || first.Status != domain.ActivationStatusOpen {
		t.Fatalf("first = %+v voided=%v err=%v", first, voided, err)
	}
	// Not overdue: refused, nothing changes.
	if _, _, err := acts.CreateReplacingOverdue(ctx, tenants.A.Scope, params(now.Add(time.Hour)), now); !errors.Is(err, repository.ErrActivationOpen) {
		t.Fatalf("second while open: err = %v, want ErrActivationOpen", err)
	}
	// Other tenant cannot create on this page.
	if _, _, err := acts.CreateReplacingOverdue(ctx, tenants.B.Scope, params(now.Add(time.Hour)), now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant create: err = %v, want ErrNoRows", err)
	}
	// Once overdue, replaced in one step.
	later := now.Add(2 * time.Hour)
	second, voided, err := acts.CreateReplacingOverdue(ctx, tenants.A.Scope, params(later.Add(time.Hour)), later)
	if err != nil || voided == nil || voided.ID != first.ID || voided.Status != domain.ActivationStatusVoid {
		t.Fatalf("replace: second=%+v voided=%+v err=%v", second, voided, err)
	}
	if open, err := acts.FindOpenByPage(ctx, tenants.A.Scope, page.ID); err != nil || open.ID != second.ID {
		t.Fatalf("open after replace = %+v err=%v", open, err)
	}

	// SetInvoice requires an invoice row of the same org (composite FK).
	invoiceID := insertActivationInvoice(t, db, tenants.A.OrganizationID, page.ID)
	if _, err := acts.SetInvoice(ctx, tenants.B.Scope, second.ID, invoiceID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant SetInvoice: err = %v", err)
	}
	linked, err := acts.SetInvoice(ctx, tenants.A.Scope, second.ID, invoiceID)
	if err != nil || linked.InvoiceID == nil || *linked.InvoiceID != invoiceID {
		t.Fatalf("SetInvoice = %+v err=%v", linked, err)
	}
	if _, err := acts.SetInvoice(ctx, tenants.A.Scope, second.ID, invoiceID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("SetInvoice twice: err = %v, want ErrNoRows", err)
	}
	if ok, err := acts.ExistsForInvoice(ctx, tenants.A.Scope, invoiceID); err != nil || !ok {
		t.Fatalf("ExistsForInvoice(A) = %v, %v", ok, err)
	}
	if ok, err := acts.ExistsForInvoice(ctx, tenants.B.Scope, invoiceID); err != nil || ok {
		t.Fatalf("ExistsForInvoice(B) = %v, %v; want false", ok, err)
	}
}
