package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/repository"
)

func TestLogHandlerListLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := NewLogHandler(&fakeLogService{
		logs: []domain.NotificationLog{sampleNotificationLog("log-1", domain.LogStatusSent)},
	}, nil)
	handler.RegisterRoutes(router.Group(""))

	req := httptest.NewRequest(http.MethodGet, "/admin/notification-logs?status=sent&limit=10&offset=5", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"count":1`) {
		t.Fatalf("response body = %s", rec.Body.String())
	}
}

func TestLogHandlerGetLog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := NewLogHandler(&fakeLogService{
		log: sampleNotificationLog("log-1", domain.LogStatusFailed),
	}, nil)
	handler.RegisterRoutes(router.Group(""))

	req := httptest.NewRequest(http.MethodGet, "/admin/notification-logs/log-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"failed"`) {
		t.Fatalf("response body = %s", rec.Body.String())
	}
}

func TestLogHandlerRetryLog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	service := &fakeLogService{log: sampleNotificationLog("log-1", domain.LogStatusSent)}
	handler := NewLogHandler(service, nil)
	handler.RegisterRoutes(router.Group(""))

	req := httptest.NewRequest(http.MethodPost, "/admin/notification-logs/log-1/retry", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.retryID != "log-1" {
		t.Fatalf("retryID = %q", service.retryID)
	}
}

func TestLogHandlerCancelLog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	service := &fakeLogService{log: sampleNotificationLog("log-1", domain.LogStatusCancelled)}
	handler := NewLogHandler(service, nil)
	handler.RegisterRoutes(router.Group(""))

	req := httptest.NewRequest(http.MethodPost, "/admin/notification-logs/log-1/cancel", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.cancelID != "log-1" {
		t.Fatalf("cancelID = %q", service.cancelID)
	}
}

func sampleNotificationLog(id string, status domain.LogStatus) domain.NotificationLog {
	return domain.NotificationLog{
		ID:            id,
		EventType:     "payment.paid",
		TemplateCode:  "payment.paid",
		Channel:       domain.ChannelEmail,
		RecipientType: "user",
		Destination:   "budi@example.test",
		Subject:       "Payment paid",
		Body:          "Paid",
		Status:        status,
		Attempts:      1,
		MaxAttempts:   3,
	}
}

type fakeLogService struct {
	log      domain.NotificationLog
	logs     []domain.NotificationLog
	err      error
	retryID  string
	cancelID string
}

func (s *fakeLogService) ListLogs(context.Context, repository.NotificationLogListFilter) ([]domain.NotificationLog, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.logs, nil
}

func (s *fakeLogService) GetLog(context.Context, string) (domain.NotificationLog, error) {
	if s.err != nil {
		return domain.NotificationLog{}, s.err
	}
	return s.log, nil
}

func (s *fakeLogService) Retry(_ context.Context, logID string) (domain.NotificationLog, error) {
	s.retryID = logID
	if s.err != nil {
		return domain.NotificationLog{}, s.err
	}
	return s.log, nil
}

func (s *fakeLogService) Cancel(_ context.Context, logID string) (domain.NotificationLog, error) {
	s.cancelID = logID
	if s.err != nil {
		return domain.NotificationLog{}, s.err
	}
	return s.log, nil
}
