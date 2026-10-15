//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestQuotationRespondIsAtomicAndOnce(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	quotations := repository.NewQuotationRepository(db)
	responses := repository.NewQuotationResponseRepository(db)

	q := newDraft(t, quotations, tenants.A.Scope, "QUO-T-0301")
	if _, err := quotations.Send(ctx, tenants.A.Scope, q.ID, ""); err != nil {
		t.Fatal(err)
	}

	got, resp, err := responses.Respond(ctx, tenants.A.Scope, repository.RecordResponseParams{
		QuotationID: q.ID, Action: domain.QuotationResponseRevisionRequested,
		Categories: []domain.RevisionCategory{domain.RevisionPrice, domain.RevisionOther}, Note: "<b>mahal</b>",
		ResponderName: "Budi", IP: "203.0.113.5", UserAgent: "UA",
	})
	if err != nil || got.Status != domain.QuotationStatusRevisionRequested || resp.Note != "<b>mahal</b>" || len(resp.Categories) != 2 {
		t.Fatalf("got=%+v resp=%+v err=%v", got.Status, resp, err)
	}
	if _, _, err := responses.Respond(ctx, tenants.A.Scope, repository.RecordResponseParams{
		QuotationID: q.ID, Action: domain.QuotationResponseApproved, ResponderName: "Budi",
	}); !errors.Is(err, repository.ErrQuotationNotRespondable) {
		t.Fatalf("second response err=%v", err)
	}
	list, _ := responses.ListByQuotation(ctx, tenants.A.Scope, q.ID)
	if len(list) != 1 || list[0].IP != "203.0.113.5" {
		t.Fatalf("responses=%+v", list)
	}
	if other, _ := responses.ListByQuotation(ctx, tenants.B.Scope, q.ID); len(other) != 0 {
		t.Fatal("tenant isolation")
	}
	// Reject kini menerima revision_requested.
	if _, err := quotations.Reject(ctx, tenants.A.Scope, q.ID, ""); err != nil {
		t.Fatalf("reject from revision_requested: %v", err)
	}

	// Approve menyetel approved_at.
	q2 := newDraft(t, quotations, tenants.A.Scope, "QUO-T-0302")
	_, _ = quotations.Send(ctx, tenants.A.Scope, q2.ID, "")
	appr, _, err := responses.Respond(ctx, tenants.A.Scope, repository.RecordResponseParams{
		QuotationID: q2.ID, Action: domain.QuotationResponseApproved, ResponderName: "Budi",
	})
	if err != nil || appr.Status != domain.QuotationStatusApproved || appr.ApprovedAt == nil {
		t.Fatalf("approve = %+v err=%v", appr, err)
	}

	// Revise menerima revision_requested.
	q3 := newDraft(t, quotations, tenants.A.Scope, "QUO-T-0303")
	_, _ = quotations.Send(ctx, tenants.A.Scope, q3.ID, "")
	if _, _, err := responses.Respond(ctx, tenants.A.Scope, repository.RecordResponseParams{
		QuotationID: q3.ID, Action: domain.QuotationResponseRevisionRequested,
		Categories: []domain.RevisionCategory{domain.RevisionPrice}, ResponderName: "Budi",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := quotations.Revise(ctx, tenants.A.Scope, q3.ID, repository.CreateQuotationParams{
		QuotationNumber: "QUO-T-0303-R1", RevisionNo: 1, Subtotal: "100.00", DiscountTotal: "0.00", TaxTotal: "0.00", GrandTotal: "100.00",
		Items: []repository.QuotationItemInput{{Description: "A", Quantity: "1.00", UnitPrice: "100.00", LineTotal: "100.00"}},
	}); err != nil {
		t.Fatalf("revise from revision_requested: %v", err)
	}
}

func TestActivityMetadataRoundTrip(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	repo := repository.NewActivityRepository(db)

	created, err := repo.Create(ctx, tenants.A.Scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityDeal, RelatedEntityID: "33333333-3333-3333-3333-333333333333",
		Type: domain.ActivityTypeQuotationResponse, Subject: "Customer menyetujui penawaran", Status: domain.ActivityStatusCompleted,
		Metadata: map[string]any{"action": "approved", "categories": []string{"price"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByID(ctx, tenants.A.Scope, created.ID)
	if err != nil || got.Metadata["action"] != "approved" {
		t.Fatalf("got=%+v err=%v", got.Metadata, err)
	}
	plain, err := repo.Create(ctx, tenants.A.Scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityDeal, RelatedEntityID: "33333333-3333-3333-3333-333333333333",
		Type: domain.ActivityTypeNote, Subject: "catatan",
	})
	if err != nil || plain.Metadata != nil {
		t.Fatalf("plain=%+v err=%v", plain.Metadata, err)
	}
}
