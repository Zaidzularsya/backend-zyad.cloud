package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"zyad.cloud/internal/modules/landing/dto"
)

func TestPageResponseSerialization(t *testing.T) {
	now := time.Now()
	resp := dto.PageResponse{
		ID:         "some-uuid",
		Name:       "Test Page",
		Title:      "Landing",
		Slug:       "test-slug",
		PageType:   "campaign",
		Status:     "draft",
		Visibility: "public",
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		CreatedAt:  now,
		UpdatedAt:  now,
		Settings:   map[string]any{"color": "blue"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded dto.PageResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.ID != resp.ID || decoded.Name != resp.Name || decoded.Settings["color"] != "blue" {
		t.Errorf("Decoded output does not match input: %+v", decoded)
	}
}

func TestPublicSubmissionRequestUnmarshaling(t *testing.T) {
	payload := `{
		"fields": {
			"email": "visitor@example.com",
			"name": "Visitor"
		},
		"consent": true,
		"context": {
			"page_version": 2,
			"utm_source": "google"
		},
		"website": ""
	}`

	var req dto.PublicSubmissionRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if req.Fields["email"] != "visitor@example.com" {
		t.Errorf("Expected email to be visitor@example.com, got %v", req.Fields["email"])
	}
	if !req.Consent {
		t.Errorf("Expected consent to be true")
	}
	if req.Context.PageVersion != 2 || req.Context.UTMSource != "google" {
		t.Errorf("Unexpected context values: %+v", req.Context)
	}
}
