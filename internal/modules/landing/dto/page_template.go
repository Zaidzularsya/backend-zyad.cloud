package dto

import "time"

type CreatePageTemplateRequest struct {
	PageID      string `json:"page_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Slug        string `json:"slug" binding:"required"`
	Category    string `json:"category" binding:"required"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

// UpdatePageTemplateRequest: semua field opsional (PATCH parsial).
type UpdatePageTemplateRequest struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	Category     *string `json:"category"`
	SortOrder    *int    `json:"sort_order"`
	Status       *string `json:"status"`
	PlatformOnly *bool   `json:"platform_only"`
}

type PageTemplateSummary struct {
	ID           string    `json:"id"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Category     string    `json:"category"`
	SortOrder    int       `json:"sort_order"`
	PlatformOnly bool      `json:"platform_only"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PageTemplateDetail struct {
	PageTemplateSummary
	HTML string `json:"html"`
	CSS  string `json:"css"`
}

type PageTemplateAdminSummary struct {
	PageTemplateSummary
	Status       string  `json:"status"`
	Source       string  `json:"source"`
	SeedModified bool    `json:"seed_modified"`
	SourcePageID *string `json:"source_page_id"`
}

type PageTemplateCopyResponse struct {
	PageID string `json:"page_id"`
	Slug   string `json:"slug"`
}
