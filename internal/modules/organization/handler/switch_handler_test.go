package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zyad.cloud/internal/core/middleware"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/modules/organization/service"

	"github.com/gin-gonic/gin"
)

const (
	handlerUserID         = "11111111-1111-1111-1111-111111111111"
	handlerSessionID      = "22222222-2222-2222-2222-222222222222"
	handlerOrganizationID = "33333333-3333-3333-3333-333333333333"
	handlerMembershipID   = "44444444-4444-4444-4444-444444444444"
)

type handlerSwitchStore struct {
	result repository.MembershipOrganization
	params repository.SwitchOrganizationParams
}

func (s *handlerSwitchStore) ListActiveByUser(
	context.Context,
	string,
) ([]repository.MembershipOrganization, error) {
	return []repository.MembershipOrganization{s.result}, nil
}

func (s *handlerSwitchStore) FindSessionSnapshot(
	context.Context,
	string,
	string,
) (repository.SessionOrganizationSnapshot, error) {
	return repository.SessionOrganizationSnapshot{}, nil
}

func (s *handlerSwitchStore) Switch(
	_ context.Context,
	params repository.SwitchOrganizationParams,
) (repository.MembershipOrganization, error) {
	s.params = params
	return s.result, nil
}

func TestSwitchHandlerRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &handlerSwitchStore{
		result: repository.MembershipOrganization{
			Organization: serviceOrganization(),
			Membership: model.Membership{
				ID:             handlerMembershipID,
				OrganizationID: handlerOrganizationID,
				UserID:         handlerUserID,
				Status:         model.MembershipStatusActive,
				Version:        1,
			},
		},
	}
	handler := NewSwitchHandler(service.NewSwitchService(store))
	router := gin.New()
	router.Use(middleware.RequestID(), handlerAuthenticatedUser())
	handler.RegisterRoutes(router)

	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(
		listRecorder,
		httptest.NewRequest(http.MethodGet, "/users/me/organizations", nil),
	)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRecorder.Code)
	}

	switchRequest := httptest.NewRequest(
		http.MethodPost,
		"/users/me/switch-organization",
		strings.NewReader(`{"organization_id":"`+handlerOrganizationID+`"}`),
	)
	switchRequest.Header.Set("Content-Type", "application/json")
	switchRequest.Header.Set("User-Agent", "handler-test")
	switchRequest.RemoteAddr = "192.0.2.20:1234"
	switchRecorder := httptest.NewRecorder()
	router.ServeHTTP(switchRecorder, switchRequest)
	if switchRecorder.Code != http.StatusOK {
		t.Fatalf("switch status = %d body=%s", switchRecorder.Code, switchRecorder.Body.String())
	}
	if store.params.UserID != handlerUserID ||
		store.params.SessionID != handlerSessionID ||
		store.params.RequestID == "" ||
		store.params.UserAgent != "handler-test" {
		t.Fatalf("switch params = %#v", store.params)
	}
}

func handlerAuthenticatedUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.AuthenticatedUserContextKey, middleware.AuthenticatedUser{
			ID:        handlerUserID,
			SessionID: handlerSessionID,
		})
		c.Next()
	}
}

func serviceOrganization() model.Organization {
	return model.Organization{
		ID:     handlerOrganizationID,
		Slug:   "acme",
		Name:   "Acme",
		Status: "active",
		Type:   "customer",
	}
}
