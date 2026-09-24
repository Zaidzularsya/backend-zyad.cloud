//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmrepo "zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	whatsappservice "zyad.cloud/internal/modules/whatsapp/service"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestMessagingRepositoryIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	tenants := testutil.NewTenantPair(t)
	setupOrganizations(t, db, tenants)
	ctx := context.Background()
	scope := tenants.A.Scope

	session, err := repository.NewSessionRepository(db).Create(ctx, scope, repository.CreateSessionParams{
		Name: "zc_test_msg_a", Purpose: domain.SessionPurposeSales,
	})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	conversations := repository.NewConversationRepository(db)
	budi, _, err := conversations.Create(ctx, scope, repository.CreateConversationParams{
		SessionID: session.ID, ChatID: "6281234567890@c.us", PhoneNormalized: "6281234567890", ContactName: "Budi 100%",
	})
	if err != nil {
		t.Fatalf("Create conversation: %v", err)
	}
	if _, _, err := conversations.Create(ctx, scope, repository.CreateConversationParams{
		SessionID: session.ID, ChatID: "6285550000000@c.us", PhoneNormalized: "6285550000000", ContactName: "Sari",
	}); err != nil {
		t.Fatalf("Create second conversation: %v", err)
	}

	// Search: literal % must not act as a wildcard; phone search works.
	if list, total, err := conversations.List(ctx, scope, repository.ConversationListFilter{Search: "100%"}); err != nil || total != 1 || list[0].ID != budi.ID {
		t.Fatalf("search 100%% = %d, %v", total, err)
	}
	if _, total, _ := conversations.List(ctx, scope, repository.ConversationListFilter{Search: "0%"}); total != 1 {
		t.Fatalf("search 0%% total = %d, want 1 (escaped)", total)
	}
	if _, total, _ := conversations.List(ctx, scope, repository.ConversationListFilter{Search: "62855"}); total != 1 {
		t.Fatalf("phone search total = %d", total)
	}
	if _, total, _ := conversations.List(ctx, tenants.B.Scope, repository.ConversationListFilter{}); total != 0 {
		t.Fatalf("organization B sees %d conversations", total)
	}

	// Link only when unlinked; assignment and status updates.
	leadID := "11111111-2222-3333-4444-555555555555"
	linked, err := conversations.LinkEntity(ctx, scope, budi.ID, domain.RelatedEntityLead, leadID, "")
	if err != nil || linked.RelatedEntityID != leadID {
		t.Fatalf("LinkEntity = %+v, %v", linked, err)
	}
	other := "99999999-2222-3333-4444-555555555555"
	if again, _ := conversations.LinkEntity(ctx, scope, budi.ID, domain.RelatedEntityContact, other, ""); again.RelatedEntityID != leadID {
		t.Fatal("LinkEntity overwrote an existing link")
	}
	closed := domain.ConversationStatusClosed
	if updated, err := conversations.Update(ctx, scope, budi.ID, repository.UpdateConversationParams{Status: &closed}); err != nil || updated.Status != closed {
		t.Fatalf("Update status = %+v, %v", updated, err)
	}
	if list, _, _ := conversations.List(ctx, scope, repository.ConversationListFilter{RelatedEntityType: domain.RelatedEntityLead, RelatedEntityID: leadID}); len(list) != 1 {
		t.Fatalf("filter by entity = %d", len(list))
	}

	// Messages: keyset pagination newest first.
	base := time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC)
	ids := []string{}
	for i := 0; i < 5; i++ {
		message, _, err := conversations.RecordMessage(ctx, scope, budi.ID, repository.RecordMessageParams{
			WAHAMessageID: "in_" + string(rune('a'+i)), Direction: domain.MessageDirectionIn,
			Body: string(rune('a' + i)), Status: domain.MessageStatusDelivered, SentAt: base.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("RecordMessage %d: %v", i, err)
		}
		ids = append(ids, message.ID)
	}
	page, err := conversations.ListMessages(ctx, scope, budi.ID, "", 2)
	if err != nil || len(page) != 2 || page[0].Body != "e" || page[1].Body != "d" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	older, err := conversations.ListMessages(ctx, scope, budi.ID, page[1].ID, 10)
	if err != nil || len(older) != 3 || older[0].Body != "c" || older[2].Body != "a" {
		t.Fatalf("older page = %+v, %v", older, err)
	}
	if err := conversations.MarkRead(ctx, scope, budi.ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if current, _ := conversations.GetByID(ctx, scope, budi.ID); current.UnreadCount != 0 {
		t.Fatalf("unread after MarkRead = %d", current.UnreadCount)
	}

	// Pending outbound message; the webhook processor stored a copy of the
	// same WAHA id first. MarkMessageSent keeps one row.
	pending, _, err := conversations.RecordMessage(ctx, scope, budi.ID, repository.RecordMessageParams{
		Direction: domain.MessageDirectionOut, Body: "halo", Status: domain.MessageStatusPending, SentAt: base.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("RecordMessage pending: %v", err)
	}
	if _, _, err := conversations.RecordMessage(ctx, scope, budi.ID, repository.RecordMessageParams{
		WAHAMessageID: "true_out_1", Direction: domain.MessageDirectionOut, Body: "halo", Status: domain.MessageStatusSent, SentAt: base.Add(time.Hour),
	}); err != nil {
		t.Fatalf("RecordMessage duplicate: %v", err)
	}
	sent, err := conversations.MarkMessageSent(ctx, scope, pending.ID, "true_out_1")
	if err != nil || sent.Status != domain.MessageStatusSent || sent.WAHAMessageID != "true_out_1" {
		t.Fatalf("MarkMessageSent = %+v, %v", sent, err)
	}
	if found, err := conversations.FindMessageByWAHAID(ctx, scope, session.ID, "true_out_1"); err != nil || found.ID != pending.ID {
		t.Fatalf("after dedupe found = %+v, %v", found, err)
	}
	failed, err := conversations.MarkMessageFailed(ctx, scope, ids[0], "provider rejected")
	if err != nil || failed.Status != domain.MessageStatusFailed || failed.Error != "provider rejected" {
		t.Fatalf("MarkMessageFailed = %+v, %v", failed, err)
	}
	if _, err := conversations.GetMessage(ctx, tenants.B.Scope, pending.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("B GetMessage error = %v", err)
	}

	// One activity claim per conversation and day.
	day := time.Date(2026, 9, 24, 10, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	if claimed, err := conversations.ClaimActivityDay(ctx, scope, budi.ID, day); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	if claimed, _ := conversations.ClaimActivityDay(ctx, scope, budi.ID, day); claimed {
		t.Fatal("second claim on the same day succeeded")
	}
	if claimed, _ := conversations.ClaimActivityDay(ctx, scope, budi.ID, day.AddDate(0, 0, 1)); !claimed {
		t.Fatal("claim on the next day failed")
	}
}

func TestCRMGatewayRecordsWhatsAppActivityIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	tenants := testutil.NewTenantPair(t)
	setupOrganizations(t, db, tenants)
	ctx := context.Background()

	lead, err := crmrepo.NewLeadRepository(db).Create(ctx, tenants.A.Scope, crmrepo.CreateLeadParams{ContactName: "WA Activity Lead", Phone: "081277770009"})
	if err != nil {
		t.Fatalf("Create lead: %v", err)
	}
	t.Cleanup(func() { deleteScoped(t, db, tenants.A.OrganizationID, "crm_leads", lead.ID) })

	gateway := whatsappservice.NewCRMGateway(
		crmrepo.NewLeadRepository(db), crmrepo.NewContactRepository(db),
		crmrepo.NewActivityRepository(db), crmrepo.NewMemberRepository(db),
	)
	entity, err := gateway.FindEntity(ctx, tenants.A.Scope, domain.RelatedEntityLead, lead.ID)
	if err != nil || entity.Phone != "081277770009" {
		t.Fatalf("FindEntity = %+v, %v", entity, err)
	}
	if err := gateway.RecordActivity(ctx, tenants.A.Scope, whatsappservice.CRMActivityInput{
		EntityType: domain.RelatedEntityLead, EntityID: lead.ID, Subject: "Chat WhatsApp +6281277770009", Description: "test",
	}); err != nil {
		t.Fatalf("RecordActivity: %v", err)
	}

	activities, _, err := crmrepo.NewActivityRepository(db).List(ctx, tenants.A.Scope, crmrepo.ActivityListFilter{
		RelatedEntityType: "lead", RelatedEntityID: lead.ID,
	})
	if err != nil || len(activities) != 1 {
		t.Fatalf("activities = %+v, %v", activities, err)
	}
	if activities[0].Type != "whatsapp" || activities[0].Status != "completed" {
		t.Fatalf("activity = %+v", activities[0])
	}
	t.Cleanup(func() { deleteScoped(t, db, tenants.A.OrganizationID, "crm_activities", activities[0].ID) })
}
