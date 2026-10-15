//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestQuotationSendLog(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	q := newDraft(t, repository.NewQuotationRepository(db), tenants.A.Scope, "QUO-T-0301")
	sends := repository.NewQuotationSendRepository(db)

	first, err := sends.Record(ctx, tenants.A.Scope, repository.RecordQuotationSendParams{
		QuotationID: q.ID, Channel: domain.QuotationChannelEmail, Mode: domain.QuotationModeTextPDF,
		Recipient: "budi@example.com", Status: domain.QuotationSendFailed, Error: "Mailbox tidak aktif", ClientRequestID: "req-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sends.Record(ctx, tenants.A.Scope, repository.RecordQuotationSendParams{
		QuotationID: q.ID, Channel: domain.QuotationChannelEmail, Mode: domain.QuotationModeTextPDF,
		Recipient: "budi@example.com", Status: domain.QuotationSendSent, ClientRequestID: "req-1",
	}); err == nil {
		t.Fatal("duplicate client_request_id must fail")
	}
	got, err := sends.FindByClientRequest(ctx, tenants.A.Scope, q.ID, "req-1")
	if err != nil || got.ID != first.ID || got.Error != "Mailbox tidak aktif" {
		t.Fatalf("found = %+v err=%v", got, err)
	}
	if _, err := sends.FindByClientRequest(ctx, tenants.A.Scope, q.ID, "nope"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing err = %v", err)
	}
	_, _ = sends.Record(ctx, tenants.A.Scope, repository.RecordQuotationSendParams{
		QuotationID: q.ID, Channel: domain.QuotationChannelWhatsApp, Mode: domain.QuotationModePDF,
		Recipient: "6281234567890", Status: domain.QuotationSendSent, ExternalMessageID: "true_628…",
	})
	list, err := sends.ListByQuotation(ctx, tenants.A.Scope, q.ID)
	if err != nil || len(list) != 2 || list[0].Channel != domain.QuotationChannelWhatsApp {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if other, _ := sends.ListByQuotation(ctx, tenants.B.Scope, q.ID); len(other) != 0 {
		t.Fatal("tenant B must not see tenant A sends")
	}
}
