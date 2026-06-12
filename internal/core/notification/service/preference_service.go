package service

import (
	"context"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/dto"
)

type PreferenceRepository interface {
	GetUserPreferences(ctx context.Context, userID string, organizationID string) ([]domain.NotificationPreference, error)
	BulkUpsertPreferences(ctx context.Context, preferences []domain.NotificationPreference) error
}

type PreferenceService struct {
	repo PreferenceRepository
}

func NewPreferenceService(repo PreferenceRepository) *PreferenceService {
	return &PreferenceService{repo: repo}
}

func (s *PreferenceService) GetUserPreferences(ctx context.Context, userID string, organizationID string) ([]domain.NotificationPreference, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, coreerrors.New("NOTIFICATION_PREFERENCE_USER_REQUIRED", "user id is required", http.StatusBadRequest)
	}
	if s.repo == nil {
		return nil, coreerrors.New("NOTIFICATION_PREFERENCE_REPOSITORY_REQUIRED", "notification preference repository is required", http.StatusInternalServerError)
	}

	preferences, err := s.repo.GetUserPreferences(ctx, userID, organizationID)
	if err != nil {
		return nil, mapNotificationError("NOTIFICATION_PREFERENCE_LIST_FAILED", "failed to list notification preferences", err)
	}
	return preferences, nil
}

func (s *PreferenceService) UpdateUserPreferences(ctx context.Context, userID string, organizationID string, req dto.BulkPreferenceRequest) ([]domain.NotificationPreference, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, coreerrors.New("NOTIFICATION_PREFERENCE_USER_REQUIRED", "user id is required", http.StatusBadRequest)
	}
	if s.repo == nil {
		return nil, coreerrors.New("NOTIFICATION_PREFERENCE_REPOSITORY_REQUIRED", "notification preference repository is required", http.StatusInternalServerError)
	}
	if len(req.Preferences) == 0 {
		return nil, coreerrors.New("NOTIFICATION_PREFERENCE_EMPTY", "notification preferences are required", http.StatusBadRequest)
	}

	preferences := make([]domain.NotificationPreference, 0, len(req.Preferences))
	seen := make(map[string]bool, len(req.Preferences))
	for _, item := range req.Preferences {
		channel := domain.Channel(item.Channel)
		if strings.TrimSpace(item.EventType) == "" {
			return nil, coreerrors.New("NOTIFICATION_PREFERENCE_EVENT_TYPE_REQUIRED", "notification preference event type is required", http.StatusBadRequest)
		}
		if !channel.IsValid() {
			return nil, coreerrors.New("NOTIFICATION_CHANNEL_INVALID", "notification channel is invalid", http.StatusBadRequest)
		}

		key := item.EventType + ":" + item.Channel
		if seen[key] {
			return nil, coreerrors.New("NOTIFICATION_PREFERENCE_DUPLICATE", "duplicate notification preference item", http.StatusBadRequest)
		}
		seen[key] = true

		preferences = append(preferences, domain.NotificationPreference{
			UserID:         userID,
			OrganizationID: organizationID,
			EventType:      item.EventType,
			Channel:        channel,
			IsEnabled:      item.IsEnabled,
		})
	}

	if err := s.repo.BulkUpsertPreferences(ctx, preferences); err != nil {
		return nil, mapNotificationError("NOTIFICATION_PREFERENCE_UPDATE_FAILED", "failed to update notification preferences", err)
	}

	return s.repo.GetUserPreferences(ctx, userID, organizationID)
}
