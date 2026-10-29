package dto

import (
	"time"

	"zyad.cloud/internal/modules/mikrotik/domain"
)

type CreateRouterRequest struct {
	Name           string `json:"name" binding:"required,max=100"`
	Host           string `json:"host" binding:"required,max=255"`
	Port           int    `json:"port" binding:"required,min=1,max=65535"`
	Username       string `json:"username" binding:"required,max=100"`
	Password       string `json:"password" binding:"required,max=255"`
	UseTLS         bool   `json:"use_tls"`
	TLSFingerprint string `json:"tls_fingerprint" binding:"max=128"`
}

// UpdateRouterRequest is a partial update: omitted fields stay unchanged.
type UpdateRouterRequest struct {
	Name           *string `json:"name" binding:"omitempty,max=100"`
	Host           *string `json:"host" binding:"omitempty,max=255"`
	Port           *int    `json:"port" binding:"omitempty,min=1,max=65535"`
	Username       *string `json:"username" binding:"omitempty,max=100"`
	Password       *string `json:"password" binding:"omitempty,max=255"`
	UseTLS         *bool   `json:"use_tls"`
	TLSFingerprint *string `json:"tls_fingerprint" binding:"omitempty,max=128"`
}

type RouterListQuery struct {
	Page    int `form:"page"`
	PerPage int `form:"per_page"`
}

// RouterResponse never carries the password: only whether one is stored.
type RouterResponse struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Host              string     `json:"host"`
	Port              int        `json:"port"`
	APIType           string     `json:"api_type"`
	UseTLS            bool       `json:"use_tls"`
	TLSFingerprint    string     `json:"tls_fingerprint,omitempty"`
	Username          string     `json:"username"`
	HasPassword       bool       `json:"has_password"`
	IsolirReady       bool       `json:"isolir_ready"`
	IsolirReadyAt     *time.Time `json:"isolir_ready_at,omitempty"`
	IsolirProfileName string     `json:"isolir_profile_name,omitempty"`
	IsolirRedirectURL string     `json:"isolir_redirect_url,omitempty"`
	LastCheckAt       *time.Time `json:"last_check_at,omitempty"`
	LastCheckStatus   string     `json:"last_check_status,omitempty"`
	RouterOSVersion   string     `json:"routeros_version,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func RouterFromDomain(r domain.Router) RouterResponse {
	return RouterResponse{
		ID: r.ID, Name: r.Name, Host: r.Host, Port: r.Port, APIType: string(r.APIType),
		UseTLS: r.UseTLS, TLSFingerprint: r.TLSFingerprint, Username: r.Username,
		HasPassword: r.PasswordEncrypted != "",
		IsolirReady: r.IsolirReady, IsolirReadyAt: r.IsolirReadyAt,
		IsolirProfileName: r.IsolirProfileName, IsolirRedirectURL: r.IsolirRedirectURL,
		LastCheckAt: r.LastCheckAt, LastCheckStatus: r.LastCheckStatus,
		RouterOSVersion: r.RouterOSVersion, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func RouterListFromDomain(routers []domain.Router) []RouterResponse {
	out := make([]RouterResponse, 0, len(routers))
	for _, r := range routers {
		out = append(out, RouterFromDomain(r))
	}
	return out
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

func BuildMeta(page, perPage int, total int64) PaginationMeta {
	totalPages := 0
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}
	return PaginationMeta{Page: page, PerPage: perPage, Total: total, TotalPages: totalPages}
}
