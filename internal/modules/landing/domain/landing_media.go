package domain

import "time"

type MediaProcessingStatus string

const (
	MediaProcessingPending   MediaProcessingStatus = "pending"
	MediaProcessingCompleted MediaProcessingStatus = "completed"
	MediaProcessingFailed    MediaProcessingStatus = "failed"
)

func (s MediaProcessingStatus) IsValid() bool {
	switch s {
	case MediaProcessingPending, MediaProcessingCompleted, MediaProcessingFailed:
		return true
	default:
		return false
	}
}

type LandingMediaAsset struct {
	ID               string                `json:"id"`
	OrganizationID   string                `json:"organization_id"`
	StorageKey       string                `json:"storage_key"`
	Filename         string                `json:"filename"`
	MimeType         string                `json:"mime_type"`
	SizeBytes        int64                 `json:"size_bytes"`
	Width            *int                  `json:"width,omitempty"`
	Height           *int                  `json:"height,omitempty"`
	DurationSeconds  *int                  `json:"duration_seconds,omitempty"`
	AltText          string                `json:"alt_text"`
	ProcessingStatus MediaProcessingStatus `json:"processing_status"`
	// PublicURL diturunkan dari StorageKey oleh service; tidak disimpan di DB.
	PublicURL string     `json:"public_url,omitempty"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
