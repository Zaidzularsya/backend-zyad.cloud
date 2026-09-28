-- Fase 3: IMAP inbox sync state for user_mailboxes, and a directory table so
-- the worker can enumerate mailboxes across all organizations (mirrors
-- wa_session_directory in 000122).
ALTER TABLE user_mailboxes
	ADD COLUMN IF NOT EXISTS sync_cursor jsonb NOT NULL DEFAULT '{}'::jsonb,
	ADD COLUMN IF NOT EXISTS last_synced_at timestamp without time zone;

-- Maps a mailbox to its organization. Intentionally NOT under RLS: the sync
-- worker runs without tenant context and has no BYPASSRLS, so it resolves the
-- organization here before opening a tenant-scoped transaction. Only the
-- sync worker reads this table; it is never exposed through tenant endpoints.
-- No deleted_at: unlike wa_sessions (soft-deleted), user_mailboxes.Delete()
-- hard-deletes, so ON DELETE CASCADE alone keeps this table current.
CREATE TABLE IF NOT EXISTS mailbox_directory (
	mailbox_id uuid PRIMARY KEY,
	organization_id uuid NOT NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT fk_mailbox_directory_mailbox
		FOREIGN KEY (organization_id, mailbox_id)
		REFERENCES user_mailboxes(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mailbox_directory_organization
	ON mailbox_directory(organization_id);

-- Backfill: mailboxes connected before this migration (Fase 2) have no
-- directory row yet. user_mailboxes is under FORCE RLS, so each
-- organization's rows are only visible with app.organization_id set — loop
-- over organizations rather than a single INSERT...SELECT.
DO $$
DECLARE
	org record;
BEGIN
	FOR org IN SELECT id FROM organizations LOOP
		PERFORM set_config('app.organization_id', org.id::text, true);
		INSERT INTO mailbox_directory (mailbox_id, organization_id)
		SELECT id, organization_id FROM user_mailboxes WHERE organization_id = org.id
		ON CONFLICT (mailbox_id) DO NOTHING;
	END LOOP;
	PERFORM set_config('app.organization_id', '', true);
END $$;
