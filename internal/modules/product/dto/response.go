package dto

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type FeatureResponse struct {
	ID            string `json:"id"`
	FeatureKey    string `json:"feature_key"`
	Module        string `json:"module"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	ValueType     string `json:"value_type"`
	Unit          string `json:"unit,omitempty"`
	ResetStrategy string `json:"reset_strategy"`
	IsActive      bool   `json:"is_active"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type FeatureListResponse struct {
	Items []FeatureResponse `json:"items"`
	Meta  PaginationMeta    `json:"meta"`
}
