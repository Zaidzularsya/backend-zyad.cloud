package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/service"

	"github.com/gin-gonic/gin"
)

type fakeRegistrar struct {
	calls int
	req   dto.RegisterRequest
}

func (f *fakeRegistrar) Register(_ context.Context, req dto.RegisterRequest, _ service.LoginHistoryRecord) (dto.GoogleAuthResponse, error) {
	f.calls++
	f.req = req
	return dto.GoogleAuthResponse{IsNewUser: true}, nil
}

func postRegister(fake *fakeRegistrar, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{registrar: fake} // nil redis: rate limit fails open
	router := gin.New()
	h.RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRegisterHandlerBindsAttributionAndIntent(t *testing.T) {
	fake := &fakeRegistrar{}
	rec := postRegister(fake, `{"name":"Budi","email":"budi@x.test","password":"secret-password","intent":"social","attribution":{"code":"ABCD2345","channel":"instagram"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.req.Intent != "social" || fake.req.Attribution == nil || fake.req.Attribution.Code != "ABCD2345" {
		t.Fatalf("req = %+v", fake.req)
	}
}

func TestRegisterHandlerRejectsInvalidEmail(t *testing.T) {
	fake := &fakeRegistrar{}
	rec := postRegister(fake, `{"name":"Budi","email":"nope","password":"secret-password"}`)
	if rec.Code != http.StatusUnprocessableEntity || fake.calls != 0 {
		t.Fatalf("status = %d calls=%d", rec.Code, fake.calls)
	}
}
