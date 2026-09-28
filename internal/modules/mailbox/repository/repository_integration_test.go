//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func setupFixtures(t *testing.T, db *database.Pool, tenants testutil.TenantPair) (userA, userB string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'mailbox-org-a', 'Mailbox Org A', 'active'),
		       ($2, 'customer', 'mailbox-org-b', 'Mailbox Org B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID, tenants.B.OrganizationID); err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	newUser := func(name string) string {
		var id string
		if err := db.QueryRow(ctx, `
			INSERT INTO users (name, email, status, created_at, updated_at)
			VALUES ('Mailbox Test', $1, 'active', now(), now())
			RETURNING id::text
		`, name+"-"+suffix+"@test.local").Scan(&id); err != nil {
			t.Fatalf("create user: %v", err)
		}
		t.Cleanup(func() { _, _ = db.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id) })
		return id
	}
	userA, userB = newUser("mailbox-a"), newUser("mailbox-b")

	cleanup := func() {
		// user_mailboxes is under FORCE RLS; delete per organization scope.
		for _, orgID := range []string{tenants.A.OrganizationID, tenants.B.OrganizationID} {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Errorf("cleanup begin: %v", err)
				return
			}
			_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
			_, _ = tx.Exec(ctx, "DELETE FROM user_mailboxes WHERE organization_id = $1", orgID)
			_ = tx.Commit(ctx)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	return userA, userB
}

func TestMailboxRepositoriesIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	tenants := testutil.NewTenantPair(t)
	userA, userB := setupFixtures(t, db, tenants)
	ctx := context.Background()
	scope := tenants.A.Scope

	mailboxes := repository.NewMailboxRepository(db)
	params := repository.MailboxParams{
		EmailAddress: "sales@zyad.test", Username: "sales@zyad.test", SecretEncrypted: "enc-1",
		SMTPHost: "smtp.zyad.test", SMTPPort: 587, SMTPSecurity: domain.SecuritySTARTTLS,
	}
	mailbox, err := mailboxes.Create(ctx, scope, userA, params)
	if err != nil {
		t.Fatalf("Create mailbox: %v", err)
	}
	if _, err := mailboxes.Create(ctx, scope, userA, params); !errors.Is(err, repository.ErrDuplicateMailbox) {
		t.Fatalf("duplicate Create error = %v, want ErrDuplicateMailbox", err)
	}
	if _, err := mailboxes.GetForUser(ctx, scope, userB, mailbox.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other user's GetForUser error = %v, want ErrNoRows", err)
	}
	if list, _ := mailboxes.ListByUser(ctx, tenants.B.Scope, userA); len(list) != 0 {
		t.Fatalf("organization B sees %d mailboxes", len(list))
	}

	// Update without a new secret keeps the stored one.
	params.DisplayName, params.SecretEncrypted = "Sales Team", ""
	updated, err := mailboxes.Update(ctx, scope, userA, mailbox.ID, params)
	if err != nil || updated.DisplayName != "Sales Team" || updated.SecretEncrypted != "enc-1" {
		t.Fatalf("Update = %+v, %v", updated, err)
	}

	messages := repository.NewMessageRepository(db)
	create := func(key string, to []string) domain.Message {
		t.Helper()
		message, _, err := messages.CreateOutbound(ctx, scope, repository.CreateOutboundParams{
			MailboxID: mailbox.ID, ClientRequestID: key, MessageID: "<" + key + "@zyad.test>",
			FromAddress: "sales@zyad.test", To: to, Bcc: []string{"Hidden@Example.com"}, Subject: "Hi " + key,
			RelatedEntityType: domain.RelatedEntityContact, RelatedEntityID: "11111111-2222-3333-4444-555555555555",
			CreatedBy: userA,
		})
		if err != nil {
			t.Fatalf("CreateOutbound(%s): %v", key, err)
		}
		return message
	}
	first := create("k1", []string{"lisa@example.com"})
	create("k2", []string{"budi@example.com"})

	again, created, err := messages.CreateOutbound(ctx, scope, repository.CreateOutboundParams{
		MailboxID: mailbox.ID, ClientRequestID: "k1", MessageID: "<other@zyad.test>", FromAddress: "sales@zyad.test", Subject: "x",
	})
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("idempotent CreateOutbound = %s created=%v %v; want %s", again.ID, created, err, first.ID)
	}

	if list, total, err := messages.List(ctx, scope, repository.MessageListFilter{UserID: userA, Participant: "LISA@example.com", Limit: 10}); err != nil || total != 1 || list[0].ID != first.ID {
		t.Fatalf("participant filter = %d, %v", total, err)
	}
	if _, total, _ := messages.List(ctx, scope, repository.MessageListFilter{UserID: userA, Participant: "hidden@example.com", Limit: 10}); total != 2 {
		t.Fatalf("bcc participant total = %d, want 2", total)
	}
	if _, total, _ := messages.List(ctx, scope, repository.MessageListFilter{UserID: userB, Limit: 10}); total != 0 {
		t.Fatalf("user B sees %d messages of user A", total)
	}

	if err := messages.MarkSent(ctx, scope, first.ID, time.Now()); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if err := messages.FailStaleQueued(ctx, scope, userA, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("FailStaleQueued: %v", err)
	}
	got, err := messages.GetForUser(ctx, scope, userA, first.ID)
	if err != nil || got.Status != domain.MessageStatusSent || got.SentAt == nil {
		t.Fatalf("sent message = %+v, %v", got, err)
	}
	list, _, _ := messages.List(ctx, scope, repository.MessageListFilter{UserID: userA, Participant: "budi@example.com", Limit: 10})
	if len(list) != 1 || list[0].Status != domain.MessageStatusFailed {
		t.Fatalf("stale queued message = %+v, want failed", list)
	}

	if err := mailboxes.Delete(ctx, scope, userB, mailbox.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other user's Delete error = %v, want ErrNoRows", err)
	}
	if err := mailboxes.Delete(ctx, scope, userA, mailbox.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, total, _ := messages.List(ctx, scope, repository.MessageListFilter{UserID: userA, Limit: 10}); total != 0 {
		t.Fatalf("messages left after mailbox delete: %d", total)
	}
}
