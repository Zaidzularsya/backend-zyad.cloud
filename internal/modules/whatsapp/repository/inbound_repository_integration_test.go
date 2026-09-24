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
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestWebhookEventRepositoryIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	events := repository.NewWebhookEventRepository(db)
	eventID := "evt_" + time.Now().Format("20060102150405.000000")
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), "DELETE FROM wa_webhook_events WHERE event_id LIKE 'evt_%'")
	})

	params := repository.InsertWebhookEventParams{
		EventID: eventID, SessionName: "zc_test_a1", EventType: "message.any",
		Payload: []byte(`{"id":"` + eventID + `","event":"message.any","session":"zc_test_a1"}`),
	}
	if inserted, err := events.Insert(ctx, params); err != nil || !inserted {
		t.Fatalf("Insert = %v, %v", inserted, err)
	}
	if inserted, err := events.Insert(ctx, params); err != nil || inserted {
		t.Fatalf("duplicate Insert = %v, %v; want false", inserted, err)
	}

	claimed, err := events.ClaimDue(ctx, 200, time.Minute)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	var mine *domain.WebhookEvent
	for i := range claimed {
		if claimed[i].EventID == eventID {
			mine = &claimed[i]
		}
	}
	if mine == nil || mine.Attempts != 1 {
		t.Fatalf("claimed event = %+v", mine)
	}

	// Leased: a second claimer must not see it.
	again, err := events.ClaimDue(ctx, 200, time.Minute)
	if err != nil {
		t.Fatalf("second ClaimDue: %v", err)
	}
	for _, event := range again {
		if event.EventID == eventID {
			t.Fatal("leased event was claimed twice")
		}
	}

	// Failed with an immediate retry: claimable again, attempt counted.
	if err := events.MarkFailed(ctx, mine.ID, "boom", time.Now().Add(-time.Second)); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	retried, _ := events.ClaimDue(ctx, 200, time.Minute)
	found := false
	for _, event := range retried {
		if event.EventID == eventID {
			found = event.Attempts == 2 && event.Error == "boom"
		}
	}
	if !found {
		t.Fatalf("retry claim = %+v", retried)
	}

	if err := events.MarkProcessed(ctx, mine.ID); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}
	var processed bool
	if err := db.QueryRow(ctx, `SELECT processed_at IS NOT NULL AND error IS NULL FROM wa_webhook_events WHERE id = $1`, mine.ID).Scan(&processed); err != nil || !processed {
		t.Fatalf("processed = %v, %v", processed, err)
	}
}

func TestConversationRepositoryIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	tenants := testutil.NewTenantPair(t)
	setupOrganizations(t, db, tenants)
	ctx := context.Background()

	session, err := repository.NewSessionRepository(db).Create(ctx, tenants.A.Scope, repository.CreateSessionParams{
		Name: "zc_test_conv_a", Purpose: domain.SessionPurposeSales,
	})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	conversations := repository.NewConversationRepository(db)

	params := repository.CreateConversationParams{
		SessionID: session.ID, ChatID: "6281234567890@c.us", PhoneNormalized: "6281234567890", ContactName: "Budi",
	}
	conversation, created, err := conversations.Create(ctx, tenants.A.Scope, params)
	if err != nil || !created {
		t.Fatalf("Create = %v, %v", created, err)
	}
	again, created, err := conversations.Create(ctx, tenants.A.Scope, params)
	if err != nil || created || again.ID != conversation.ID {
		t.Fatalf("second Create = %+v, %v, %v; want existing", again, created, err)
	}
	if _, err := conversations.GetBySessionChat(ctx, tenants.B.Scope, session.ID, params.ChatID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("B GetBySessionChat error = %v, want ErrNoRows", err)
	}

	sentAt := time.Date(2026, 9, 24, 3, 0, 0, 0, time.UTC)
	message := repository.RecordMessageParams{
		WAHAMessageID: "false_6281234567890@c.us_A1", Direction: domain.MessageDirectionIn, Body: "halo",
		Preview: "halo", Status: domain.MessageStatusDelivered, SentAt: sentAt, Raw: map[string]any{"id": "x"},
	}
	if _, inserted, err := conversations.RecordMessage(ctx, tenants.A.Scope, conversation.ID, message); err != nil || !inserted {
		t.Fatalf("RecordMessage = %v, %v", inserted, err)
	}
	if _, inserted, err := conversations.RecordMessage(ctx, tenants.A.Scope, conversation.ID, message); err != nil || inserted {
		t.Fatalf("duplicate RecordMessage = %v, %v; want false", inserted, err)
	}
	// An older outbound message must not overwrite the preview or add unread.
	older := message
	older.WAHAMessageID, older.Direction, older.Body, older.Preview = "true_x_B1", domain.MessageDirectionOut, "lama", "lama"
	older.Status, older.SentAt = domain.MessageStatusSent, sentAt.Add(-time.Hour)
	if _, inserted, err := conversations.RecordMessage(ctx, tenants.A.Scope, conversation.ID, older); err != nil || !inserted {
		t.Fatalf("RecordMessage older = %v, %v", inserted, err)
	}

	current, err := conversations.GetBySessionChat(ctx, tenants.A.Scope, session.ID, params.ChatID)
	if err != nil {
		t.Fatalf("GetBySessionChat: %v", err)
	}
	if current.UnreadCount != 1 || current.LastMessagePreview != "halo" || current.LastMessageAt == nil || !current.LastMessageAt.Equal(sentAt) {
		t.Fatalf("conversation = %+v", current)
	}

	outbound, err := conversations.FindMessageByWAHAID(ctx, tenants.A.Scope, session.ID, "true_x_B1")
	if err != nil || outbound.Direction != domain.MessageDirectionOut {
		t.Fatalf("FindMessageByWAHAID = %+v, %v", outbound, err)
	}
	if updated, err := conversations.SetMessageStatus(ctx, tenants.A.Scope, outbound.ID, domain.MessageStatusSent, domain.MessageStatusRead); err != nil || !updated {
		t.Fatalf("SetMessageStatus = %v, %v", updated, err)
	}
	if updated, err := conversations.SetMessageStatus(ctx, tenants.A.Scope, outbound.ID, domain.MessageStatusSent, domain.MessageStatusDelivered); err != nil || updated {
		t.Fatalf("stale SetMessageStatus = %v, %v; want false", updated, err)
	}
}

func TestCRMFindActiveByPhoneIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	tenants := testutil.NewTenantPair(t)
	setupOrganizations(t, db, tenants)
	ctx := context.Background()
	leads := crmrepo.NewLeadRepository(db)
	contacts := crmrepo.NewContactRepository(db)

	lead, err := leads.Create(ctx, tenants.A.Scope, crmrepo.CreateLeadParams{ContactName: "WA Test Lead", Phone: "0812-7777-0001"})
	if err != nil {
		t.Fatalf("Create lead: %v", err)
	}
	contact, err := contacts.Create(ctx, tenants.A.Scope, crmrepo.CreateContactParams{FirstName: "WA", LastName: "Contact", Phone: "+62 812 7777 0002"})
	if err != nil {
		t.Fatalf("Create contact: %v", err)
	}
	t.Cleanup(func() {
		deleteScoped(t, db, tenants.A.OrganizationID, "crm_leads", lead.ID)
		deleteScoped(t, db, tenants.A.OrganizationID, "crm_contacts", contact.ID)
	})

	found, err := leads.FindActiveByPhone(ctx, tenants.A.Scope, "6281277770001")
	if err != nil || found.ID != lead.ID {
		t.Fatalf("FindActiveByPhone lead = %+v, %v", found, err)
	}
	if _, err := leads.FindActiveByPhone(ctx, tenants.B.Scope, "6281277770001"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("B lead lookup error = %v, want ErrNoRows", err)
	}
	foundContact, err := contacts.FindActiveByPhone(ctx, tenants.A.Scope, "6281277770002")
	if err != nil || foundContact.ID != contact.ID {
		t.Fatalf("FindActiveByPhone contact = %+v, %v", foundContact, err)
	}
}

func TestOwnerRepositoryIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	// Any organization that has an active owner in the test database.
	var orgID string
	err := db.QueryRow(ctx, `
		SELECT m.organization_id::text
		FROM organization_memberships m
		JOIN user_roles ur ON ur.user_id = m.user_id AND ur.organization_id = m.organization_id
		JOIN roles r ON r.id = ur.role_id
		WHERE m.status = 'active' AND r.slug = 'organization_owner'
		LIMIT 1
	`).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		t.Skip("no organization with an active owner in the test database")
	}
	if err != nil {
		t.Fatalf("find organization: %v", err)
	}

	owners, err := repository.NewOwnerRepository(db).ListOrganizationOwners(ctx, orgID)
	if err != nil || len(owners) == 0 {
		t.Fatalf("ListOrganizationOwners = %+v, %v", owners, err)
	}
	for _, owner := range owners {
		if owner.Email == "" || owner.UserID == "" {
			t.Fatalf("owner = %+v", owner)
		}
	}
}

// deleteScoped hard-deletes a test row from an RLS table under its organization.
func deleteScoped(t *testing.T, db *database.Pool, organizationID, table, id string) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Errorf("cleanup begin: %v", err)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", organizationID); err != nil {
		t.Errorf("cleanup set_config: %v", err)
		return
	}
	if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE id = $1", id); err != nil {
		t.Errorf("cleanup %s: %v", table, err)
		return
	}
	_ = tx.Commit(ctx)
}
