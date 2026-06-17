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
	ID               string
	OrganizationID   string
	StorageKey       string
	Filename         string
	MimeType         string
	SizeBytes        int64
	Width            *int
	Height           *int
	DurationSeconds  *int
	AltText          string
	ProcessingStatus MediaProcessingStatus
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}
