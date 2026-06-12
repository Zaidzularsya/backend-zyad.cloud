package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/dto"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
)

func TestPreferenceHandlerGetMyPreferences(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := NewPreferenceHandler(&fakePreferenceService{
		preferences: []domain.NotificationPreference{
			{UserID: "user-1", EventType: "payment.paid", Channel: domain.ChannelEmail, IsEnabled: true},
		},
	}, nil)
	group := router.Group("")
	group.Use(func(c *gin.Context) {
		permissionmiddleware.SetUserID(c, "user-1")
		c.Next()
	})
	handler.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/users/me/notification-preferences?organization_id=org-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"event_type":"payment.paid"`) {
		t.Fatalf("response body = %s", rec.Body.String())
	}
}

func TestPreferenceHandlerUpdateMyPreferences(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	service := &fakePreferenceService{}
	handler := NewPreferenceHandler(service, nil)
	group := router.Group("")
	group.Use(func(c *gin.Context) {
		permissionmiddleware.SetUserID(c, "user-1")
		c.Next()
	})
	handler.RegisterRoutes(group)

	body := `{"preferences":[{"event_type":"payment.paid","channel":"email","is_enabled":false}]}`
	req := httptest.NewRequest(http.MethodPatch, "/users/me/notification-preferences", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.updatedUserID != "user-1" {
		t.Fatalf("updatedUserID = %q", service.updatedUserID)
	}
}

func TestPreferenceHandlerUpdateAdminUserPreferences(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	service := &fakePreferenceService{}
	handler := NewPreferenceHandler(service, nil)
	handler.RegisterRoutes(router.Group(""))

	body := `{"preferences":[{"event_type":"payment.paid","channel":"whatsapp","is_enabled":true}]}`
	req := httptest.NewRequest(http.MethodPatch, "/admin/users/user-2/notification-preferences?organization_id=org-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.updatedUserID != "user-2" || service.updatedOrganizationID != "org-1" {
		t.Fatalf("updated user/org = %q/%q", service.updatedUserID, service.updatedOrganizationID)
	}
}

type fakePreferenceService struct {
	preferences           []domain.NotificationPreference
	err                   error
	updatedUserID         string
	updatedOrganizationID string
}

func (s *fakePreferenceService) GetUserPreferences(context.Context, string, string) ([]domain.NotificationPreference, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.preferences, nil
}

func (s *fakePreferenceService) UpdateUserPreferences(_ context.Context, userID string, organizationID string, req dto.BulkPreferenceRequest) ([]domain.NotificationPreference, error) {
	s.updatedUserID = userID
	s.updatedOrganizationID = organizationID
	if s.err != nil {
		return nil, s.err
	}
	preferences := make([]domain.NotificationPreference, 0, len(req.Preferences))
	for _, item := range req.Preferences {
		preferences = append(preferences, domain.NotificationPreference{
			UserID:         userID,
			OrganizationID: organizationID,
			EventType:      item.EventType,
			Channel:        domain.Channel(item.Channel),
			IsEnabled:      item.IsEnabled,
		})
	}
	s.preferences = preferences
	return preferences, nil
}
