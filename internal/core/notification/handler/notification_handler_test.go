package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/notification/domain"
)

func TestNotificationHandlerSendByTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := NewNotificationHandler(&fakeNotificationSender{
		log: domain.NotificationLog{
			ID:            "log-1",
			EventType:     "payment.paid",
			TemplateCode:  "payment.paid",
			Channel:       domain.ChannelEmail,
			RecipientType: "user",
			Destination:   "budi@example.test",
			Subject:       "Payment INV-2026-0001",
			Body:          "Paid",
			Status:        domain.LogStatusSent,
			Attempts:      0,
			MaxAttempts:   3,
		},
	})
	handler.RegisterInternalRoutes(router.Group(""))

	body := `{
		"event_type": "payment.paid",
		"template_code": "payment.paid",
		"channel": "email",
		"user_id": "user-1",
		"recipient": {
			"type": "user",
			"user_id": "user-1",
			"name": "Budi",
			"email": "budi@example.test",
			"destination": "budi@example.test"
		},
		"payload": {
			"app_name": "Zyad Cloud",
			"customer_name": "Budi",
			"invoice_number": "INV-2026-0001",
			"amount": "Rp150.000",
			"paid_at": "2026-06-12 10:00 WIB"
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/internal/notifications/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"log-1"`) {
		t.Fatalf("response body = %s", rec.Body.String())
	}
}

type fakeNotificationSender struct {
	log domain.NotificationLog
	err error
}

func (s *fakeNotificationSender) SendByTemplate(context.Context, domain.Notification) (domain.NotificationLog, error) {
	if s.err != nil {
		return domain.NotificationLog{}, s.err
	}
	return s.log, nil
}
