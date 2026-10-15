package repository

import (
	"context"

	"zyad.cloud/internal/platform/database"
)

type directoryRepository struct {
	db *database.Pool
}

func NewDirectoryRepository(db *database.Pool) DirectoryRepository {
	return &directoryRepository{db: db}
}

func (r *directoryRepository) ListAll(ctx context.Context) ([]DirectoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT mailbox_id, organization_id FROM mailbox_directory ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []DirectoryEntry{}
	for rows.Next() {
		var entry DirectoryEntry
		if err := rows.Scan(&entry.MailboxID, &entry.OrganizationID); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
