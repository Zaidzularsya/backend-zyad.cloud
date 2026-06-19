package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"zyad.cloud/internal/config"
	coreauth "zyad.cloud/internal/core/auth"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/redis"
)

var (
	ErrPageNotFound       = errors.New("landing page not found")
	ErrInvalidVisibility  = errors.New("invalid page visibility")
	ErrPasswordRequired   = errors.New("password is required for password_protected visibility")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrTooManyAttempts    = errors.New("too many access attempts, try again later")
	ErrTokenManagerFailed = errors.New("failed to issue access token")
)

type visibilityService struct {
	pageRepo     repository.PageRepository
	platformRepo repository.PlatformPageRepository
	redis        *redis.Client
	tokenMgr     *coreauth.TokenManager
}

func NewVisibilityService(
	pageRepo repository.PageRepository,
	platformRepo repository.PlatformPageRepository,
	redisClient *redis.Client,
	cfg config.Config,
) VisibilityService {
	// Initialize token manager with app secret or auth secret
	secret := cfg.Auth.Secret
	if secret == "" {
		secret = cfg.App.Secret
	}
	tokenMgr, _ := coreauth.NewTokenManager(secret, "landing_visibility")
	
	return &visibilityService{
		pageRepo:     pageRepo,
		platformRepo: platformRepo,
		redis:        redisClient,
		tokenMgr:     tokenMgr,
	}
}

func (s *visibilityService) UpdateVisibility(ctx context.Context, scope coretenant.Scope, pageID string, visibility domain.PageVisibility, updatedBy string) error {
	if !visibility.IsValid() {
		return ErrInvalidVisibility
	}

	page, err := s.pageRepo.FindByID(ctx, scope, pageID)
	if err != nil {
		return fmt.Errorf("find page: %w", err)
	}

	if visibility == domain.PageVisibilityPasswordProtected && page.PasswordHash == "" {
		return ErrPasswordRequired
	}

	_, err = s.pageRepo.Update(ctx, scope, pageID, repository.UpdatePageParams{
		Visibility: &visibility,
		UpdatedBy:  updatedBy,
	})
	if err != nil {
		return fmt.Errorf("update page visibility: %w", err)
	}

	return nil
}

func (s *visibilityService) SetPassword(ctx context.Context, scope coretenant.Scope, pageID string, plainPassword string, updatedBy string) error {
	if plainPassword == "" {
		return ErrInvalidPassword
	}

	_, err := s.pageRepo.FindByID(ctx, scope, pageID)
	if err != nil {
		return fmt.Errorf("find page: %w", err)
	}

	hash, err := coreauth.HashPassword(plainPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	visibility := domain.PageVisibilityPasswordProtected
	_, err = s.pageRepo.Update(ctx, scope, pageID, repository.UpdatePageParams{
		Visibility:   &visibility,
		PasswordHash: &hash,
		UpdatedBy:    updatedBy,
	})
	if err != nil {
		return fmt.Errorf("update page password: %w", err)
	}

	return nil
}

func (s *visibilityService) RemovePassword(ctx context.Context, scope coretenant.Scope, pageID string, updatedBy string) error {
	page, err := s.pageRepo.FindByID(ctx, scope, pageID)
	if err != nil {
		return fmt.Errorf("find page: %w", err)
	}

	// If page is currently password protected, fallback to private
	var newVisibility *domain.PageVisibility
	if page.Visibility == domain.PageVisibilityPasswordProtected {
		privateVis := domain.PageVisibilityPrivate
		newVisibility = &privateVis
	}

	emptyHash := ""
	_, err = s.pageRepo.Update(ctx, scope, pageID, repository.UpdatePageParams{
		Visibility:   newVisibility,
		PasswordHash: &emptyHash,
		UpdatedBy:    updatedBy,
	})
	if err != nil {
		return fmt.Errorf("remove page password: %w", err)
	}

	return nil
}

func (s *visibilityService) VerifyAccess(ctx context.Context, organizationID string, pageID string, submittedPassword string, ipAddress string) (string, error) {
	// Check rate limit if redis is available
	if s.redis != nil && ipAddress != "" {
		rateLimitKey := fmt.Sprintf("landing:access_attempt:%s:%s", pageID, ipAddress)
		count, err := s.redis.Incr(ctx, rateLimitKey).Result()
		if err == nil {
			if count == 1 {
				s.redis.Expire(ctx, rateLimitKey, 15*time.Minute)
			}
			if count > 10 {
				return "", ErrTooManyAttempts
			}
		}
	}

	page, err := s.platformRepo.FindByOrganizationAndID(ctx, organizationID, pageID)
	if err != nil {
		return "", fmt.Errorf("find page: %w", err)
	}

	if page.Visibility != domain.PageVisibilityPasswordProtected {
		// If not password protected, no need to verify access this way
		return "", nil
	}

	if !coreauth.VerifyPassword(submittedPassword, page.PasswordHash) {
		return "", ErrInvalidPassword
	}

	// Password matches, issue short-lived access grant (e.g. 1 hour)
	if s.tokenMgr == nil {
		return "", ErrTokenManagerFailed
	}
	
	claims := coreauth.Claims{
		Subject: pageID,
	}
	token, err := s.tokenMgr.Generate(claims, 1*time.Hour)
	if err != nil {
		return "", fmt.Errorf("generate access token: %w", err)
	}

	return token, nil
}

func (s *visibilityService) ValidateAccessGrant(ctx context.Context, pageID string, token string) error {
	if s.tokenMgr == nil {
		return ErrTokenManagerFailed
	}
	claims, err := s.tokenMgr.Parse(token)
	if err != nil {
		return err
	}
	if claims.Subject != pageID {
		return coreauth.ErrInvalidToken
	}
	return nil
}
