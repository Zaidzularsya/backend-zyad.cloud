package domain

import "time"

// LeadAttachment menautkan satu file storage tenant (asset_objects) ke lead.
// Filename/MimeType/SizeBytes dibaca dari asset_objects lewat join.
type LeadAttachment struct {
	ID            string
	LeadID        string
	AssetObjectID string
	Filename      string
	MimeType      string
	SizeBytes     int64
	CreatedBy     string
	CreatedAt     time.Time
}
