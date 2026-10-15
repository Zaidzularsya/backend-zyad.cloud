DROP TABLE IF EXISTS mailbox_directory;

ALTER TABLE user_mailboxes
	DROP COLUMN IF EXISTS sync_cursor,
	DROP COLUMN IF EXISTS last_synced_at;
