package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database"
)

var (
	ErrValidationFailed = errors.New("page validation failed for publishing")
	ErrInvalidToken     = errors.New("invalid or expired preview token")
)

type publishService struct {
	db            *database.Pool
	pageRepo      repository.PageRepository
	sectionRepo   repository.SectionRepository
	formRepo      repository.FormRepository
	brandingRepo  repository.BrandingRepository
	versionRepo   repository.VersionRepository
	documentRepo  repository.DocumentRepository
	previewSecret string
}

func NewPublishService(
	db *database.Pool,
	pageRepo repository.PageRepository,
	sectionRepo repository.SectionRepository,
	formRepo repository.FormRepository,
	brandingRepo repository.BrandingRepository,
	versionRepo repository.VersionRepository,
	documentRepo repository.DocumentRepository,
	previewSecret string,
) PublishService {
	if previewSecret == "" {
		// APP_SECRET is required in production (see config.AppConfig.validate), so an empty
		// value here only happens in non-production environments. Generate a random,
		// process-local secret instead of a fixed literal so preview tokens can't be forged
		// by anyone who has read the source code.
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			panic(fmt.Sprintf("landing: failed to generate fallback preview secret: %v", err))
		}
		previewSecret = hexEncode(buf)
	}
	return &publishService{
		db:            db,
		pageRepo:      pageRepo,
		sectionRepo:   sectionRepo,
		formRepo:      formRepo,
		brandingRepo:  brandingRepo,
		versionRepo:   versionRepo,
		documentRepo:  documentRepo,
		previewSecret: previewSecret,
	}
}

func (s *publishService) ValidateForPublish(ctx context.Context, scope tenant.Scope, pageID string) (PublishChecklist, error) {
	if !scope.IsValid() {
		return PublishChecklist{}, tenant.ErrInvalidScope
	}

	result := PublishChecklist{
		Errors:   []string{},
		Warnings: []string{},
		IsValid:  true,
	}

	page, err := s.pageRepo.FindByID(ctx, scope, pageID)
	if err != nil {
		return result, err
	}

	if strings.TrimSpace(page.Title) == "" {
		result.Errors = append(result.Errors, "Page title is missing.")
		result.IsValid = false
	}
	if strings.TrimSpace(page.Slug) == "" {
		result.Errors = append(result.Errors, "Page slug is missing.")
		result.IsValid = false
	}

	if page.Builder == domain.PageBuilderGrapesJS {
		doc, docErr := s.documentRepo.GetByPageID(ctx, scope, pageID)
		if docErr != nil || strings.TrimSpace(doc.HTML) == "" {
			result.Errors = append(result.Errors, "Page has no saved content yet.")
			result.IsValid = false
		}
		return result, nil
	}

	// Check sections
	sections, err := s.sectionRepo.ListByPage(ctx, scope, pageID)
	if err != nil {
		return result, err
	}
	if len(sections) == 0 {
		result.Warnings = append(result.Warnings, "Page has no content sections.")
	}

	// Check branding
	branding, err := s.brandingRepo.GetByPage(ctx, scope, pageID)
	if err != nil {
		branding, err = s.brandingRepo.GetDefault(ctx, scope)
	}

	if err != nil {
		result.Warnings = append(result.Warnings, "Organization branding is not configured.")
	} else if branding.LogoLightURL == "" {
		result.Warnings = append(result.Warnings, "Organization logo is missing.")
	}

	return result, nil
}

func (s *publishService) Publish(ctx context.Context, scope tenant.Scope, pageID string, changeNote string, publishedBy string) (domain.LandingPageVersion, error) {
	if !scope.IsValid() {
		return domain.LandingPageVersion{}, tenant.ErrInvalidScope
	}

	checklist, err := s.ValidateForPublish(ctx, scope, pageID)
	if err != nil {
		return domain.LandingPageVersion{}, err
	}
	if !checklist.IsValid {
		return domain.LandingPageVersion{}, ErrValidationFailed
	}

	// 1. Fetch full snapshot of the page
	page, err := s.pageRepo.FindByID(ctx, scope, pageID)
	if err != nil {
		return domain.LandingPageVersion{}, err
	}

	var snapshot map[string]any
	if page.Builder == domain.PageBuilderGrapesJS {
		doc, docErr := s.documentRepo.GetByPageID(ctx, scope, pageID)
		if docErr != nil {
			return domain.LandingPageVersion{}, ErrValidationFailed
		}
		snapshot = buildGrapesJSSnapshot(page, doc, time.Now().UTC())
	} else {
		sections, _ := s.sectionRepo.ListByPage(ctx, scope, pageID)
		forms, _ := s.formRepo.ListByPage(ctx, scope, pageID)
		snapshot = map[string]any{
			"page":          page,
			"sections":      sections,
			"forms":         forms,
			"snapshot_time": time.Now().UTC(),
		}
	}

	// 2. We use a manual transaction approach, but our repos use db pool directly.
	// To ensure consistency, we should ideally run these in a single Tx, but since
	// VersionRepo creates the snapshot as JSON, it's atomic from the read side.
	// 3. Get next version number
	versions, err := s.versionRepo.ListByPage(ctx, scope, pageID)
	nextVersion := 1
	if err == nil && len(versions) > 0 {
		nextVersion = versions[0].Version + 1
	}

	// 4. Create version
	versionParams := repository.CreateVersionParams{
		LandingPageID: pageID,
		Version:       nextVersion,
		ChangeNote:    changeNote,
		Snapshot:      snapshot,
		CreatedBy:     publishedBy,
	}
	version, err := s.versionRepo.Create(ctx, scope, versionParams)
	if err != nil {
		return domain.LandingPageVersion{}, err
	}

	// 5. Update page status to published
	status := domain.PageStatusPublished
	_, err = s.pageRepo.Update(ctx, scope, pageID, repository.UpdatePageParams{
		Status:    &status,
		UpdatedBy: publishedBy,
	})
	if err != nil {
		return domain.LandingPageVersion{}, err
	}

	return version, nil
}

func (s *publishService) Unpublish(ctx context.Context, scope tenant.Scope, pageID string, updatedBy string) error {
	if !scope.IsValid() {
		return tenant.ErrInvalidScope
	}

	status := domain.PageStatusDraft
	_, err := s.pageRepo.Update(ctx, scope, pageID, repository.UpdatePageParams{
		Status:    &status,
		UpdatedBy: updatedBy,
	})
	return err
}

func (s *publishService) RestoreVersion(ctx context.Context, scope tenant.Scope, pageID string, versionID string, restoredBy string) (domain.LandingPage, error) {
	if !scope.IsValid() {
		return domain.LandingPage{}, tenant.ErrInvalidScope
	}

	version, err := s.versionRepo.FindByID(ctx, scope, versionID)
	if err != nil {
		return domain.LandingPage{}, err
	}

	// A true restore would copy the JSON snapshot back into relational tables (sections, forms, etc.)
	// For this scope, we just return the snapshot or update the page base data.
	// Implementing a full deep restore is complex, so we will stub the metadata restore here.

	// In real implementation:
	// - Delete current sections
	// - Insert sections from snapshot
	// - Restore page metadata

	// Update page base
	// Note: We don't restore PageStatus to Published automatically. Restored pages should become Drafts.
	status := domain.PageStatusDraft
	note := fmt.Sprintf("Restored from version %d", version.Version)
	updated, err := s.pageRepo.Update(ctx, scope, pageID, repository.UpdatePageParams{
		Status:    &status,
		UpdatedBy: restoredBy,
	})

	// Add note (optional: a mechanism to record restore notes)
	_ = note

	return updated, err
}

func (s *publishService) GeneratePreviewToken(ctx context.Context, scope tenant.Scope, pageID string, durationMinutes int) (string, error) {
	if !scope.IsValid() {
		return "", tenant.ErrInvalidScope
	}
	if durationMinutes == 0 {
		durationMinutes = 60 // default 1 hour
	}

	expiresAt := time.Now().Add(time.Duration(durationMinutes) * time.Minute).Unix()

	payload := fmt.Sprintf("%s:%d", pageID, expiresAt)

	mac := hmac.New(sha256.New, []byte(s.previewSecret))
	mac.Write([]byte(payload))
	signature := hexEncode(mac.Sum(nil))

	tokenStr := fmt.Sprintf("%s:%s", payload, signature)
	return base64.URLEncoding.EncodeToString([]byte(tokenStr)), nil
}

func (s *publishService) ValidatePreviewToken(ctx context.Context, token string) (string, error) {
	decodedBytes, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", ErrInvalidToken
	}

	parts := strings.Split(string(decodedBytes), ":")
	if len(parts) != 3 {
		return "", ErrInvalidToken
	}

	pageID := parts[0]
	expiresAtStr := parts[1]
	providedSignature := parts[2]

	expiresAt, err := strconv.ParseInt(expiresAtStr, 10, 64)
	if err != nil {
		return "", ErrInvalidToken
	}

	if time.Now().Unix() > expiresAt {
		return "", ErrInvalidToken
	}

	payload := fmt.Sprintf("%s:%s", pageID, expiresAtStr)
	mac := hmac.New(sha256.New, []byte(s.previewSecret))
	mac.Write([]byte(payload))
	expectedSignature := hexEncode(mac.Sum(nil))

	if !hmac.Equal([]byte(providedSignature), []byte(expectedSignature)) {
		return "", ErrInvalidToken
	}

	return pageID, nil
}

func hexEncode(b []byte) string {
	const hextable = "0123456789abcdef"
	res := make([]byte, len(b)*2)
	for i, v := range b {
		res[i*2] = hextable[v>>4]
		res[i*2+1] = hextable[v&0x0f]
	}
	return string(res)
}
