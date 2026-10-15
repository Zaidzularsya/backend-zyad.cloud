package domain

import "time"

// ContactAttachment menautkan satu file storage tenant (asset_objects) ke contact.
// Filename/MimeType/SizeBytes dibaca dari asset_objects lewat join.
type ContactAttachment struct {
	ID            string
	ContactID     string
	AssetObjectID string
	Filename      string
	MimeType      string
	SizeBytes     int64
	CreatedBy     string
	CreatedAt     time.Time
}
