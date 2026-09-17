package domain

import "time"

// ObjectClass mengikuti storage.ObjectClass — disalin di sini sebagai string
// biasa supaya domain package tidak perlu import internal/platform/storage.
type ObjectClass string

const (
	ObjectClassPublic  ObjectClass = "public"
	ObjectClassPrivate ObjectClass = "private"
)

func (c ObjectClass) IsValid() bool {
	switch c {
	case ObjectClassPublic, ObjectClassPrivate:
		return true
	default:
		return false
	}
}

// AssetObject adalah satu file yang diunggah tenant ke storage tenant —
// generik, tidak terikat ke landing page atau modul manapun. Dipakai untuk
// kebutuhan non-landing seperti dokumen KYC/legal (KTP, dll).
type AssetObject struct {
	ID             string      `json:"id"`
	OrganizationID string      `json:"organization_id"`
	StorageKey     string      `json:"storage_key"`
	Filename       string      `json:"filename"`
	MimeType       string      `json:"mime_type"`
	SizeBytes      int64       `json:"size_bytes"`
	Class          ObjectClass `json:"class"`
	Label          string      `json:"label,omitempty"`
	// PublicURL / DownloadURL diturunkan oleh service, tidak disimpan di DB.
	PublicURL string     `json:"public_url,omitempty"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
