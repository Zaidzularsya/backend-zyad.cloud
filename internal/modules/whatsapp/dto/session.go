package dto

import (
	"time"

	"zyad.cloud/internal/modules/whatsapp/domain"
)

type CreateSessionRequest struct {
	DisplayName    string `json:"display_name" binding:"max=120"`
	Purpose        string `json:"purpose"`
	IsDefault      bool   `json:"is_default"`
	AutoCreateLead bool   `json:"auto_create_lead"`
}

type UpdateSessionRequest struct {
	DisplayName    *string `json:"display_name" binding:"omitempty,max=120"`
	IsDefault      *bool   `json:"is_default"`
	Purpose        *string `json:"purpose"`
	AutoCreateLead *bool   `json:"auto_create_lead"`
}

type PairingCodeRequest struct {
	Phone string `json:"phone" binding:"required"`
}

// SessionResponse deliberately omits the WAHA session name: clients address
// sessions by id only.
type SessionResponse struct {
	ID             string     `json:"id"`
	DisplayName    string     `json:"display_name,omitempty"`
	Phone          string     `json:"phone,omitempty"`
	PushName       string     `json:"push_name,omitempty"`
	Status         string     `json:"status"`
	Engine         string     `json:"engine,omitempty"`
	IsDefault      bool       `json:"is_default"`
	Purpose        string     `json:"purpose"`
	AutoCreateLead bool       `json:"auto_create_lead"`
	LastStatusAt   *time.Time `json:"last_status_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func SessionFromDomain(s domain.Session) SessionResponse {
	return SessionResponse{
		ID:             s.ID,
		DisplayName:    s.DisplayName,
		Phone:          s.Phone,
		PushName:       s.PushName,
		Status:         string(s.Status),
		Engine:         s.Engine,
		IsDefault:      s.IsDefault,
		Purpose:        string(s.Purpose),
		AutoCreateLead: s.AutoCreateLead,
		LastStatusAt:   s.LastStatusAt,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

func SessionListFromDomain(sessions []domain.Session) []SessionResponse {
	items := make([]SessionResponse, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, SessionFromDomain(s))
	}
	return items
}

type SessionQRResponse struct {
	// QR is a data URL (data:image/png;base64,...) usable directly as <img src>.
	QR     string `json:"qr"`
	Status string `json:"status"`
}

type PairingCodeResponse struct {
	Code string `json:"code"`
}
