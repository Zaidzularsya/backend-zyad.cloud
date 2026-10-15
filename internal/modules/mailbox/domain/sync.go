package domain

// FolderCursor is the sync position of one IMAP folder. UIDValidity resets
// LastUID to 0 when the server reports a new value (the mailbox was
// recreated; old UIDs no longer mean anything).
type FolderCursor struct {
	UIDValidity uint32 `json:"uidvalidity"`
	LastUID     uint32 `json:"last_uid"`
}

// SyncCursor is stored as user_mailboxes.sync_cursor, keyed by folder name
// ("INBOX", "Sent", ...).
type SyncCursor map[string]FolderCursor
