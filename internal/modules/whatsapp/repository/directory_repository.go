package repository

import (
	"context"

	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/platform/database"
)

type directoryRepository struct {
	db *database.Pool
}

func NewDirectoryRepository(db *database.Pool) DirectoryRepository {
	return &directoryRepository{db: db}
}

func (r *directoryRepository) Resolve(ctx context.Context, sessionName string) (domain.DirectoryEntry, error) {
	var entry domain.DirectoryEntry
	err := r.db.QueryRow(ctx, `
		SELECT session_name, organization_id, session_id
		FROM wa_session_directory
		WHERE session_name = $1 AND deleted_at IS NULL
	`, sessionName).Scan(&entry.SessionName, &entry.OrganizationID, &entry.SessionID)
	if err != nil {
		return domain.DirectoryEntry{}, err
	}
	return entry, nil
}

func (r *directoryRepository) ListActive(ctx context.Context) ([]domain.DirectoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT session_name, organization_id, session_id
		FROM wa_session_directory
		WHERE deleted_at IS NULL
		ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []domain.DirectoryEntry{}
	for rows.Next() {
		var entry domain.DirectoryEntry
		if err := rows.Scan(&entry.SessionName, &entry.OrganizationID, &entry.SessionID); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
