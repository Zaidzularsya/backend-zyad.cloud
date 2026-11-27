package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/service"

	"github.com/gin-gonic/gin"
)

type fakeGoogleAuthenticator struct {
	calls int
	req   dto.GoogleAuthRequest
	err   error
}

func (f *fakeGoogleAuthenticator) GoogleAuth(_ context.Context, req dto.GoogleAuthRequest, _ service.LoginHistoryRecord) (dto.GoogleAuthResponse, error) {
	f.calls++
	f.req = req
	return dto.GoogleAuthResponse{}, f.err
}

func newIntentRouter(fake *fakeGoogleAuthenticator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{googleAuth: fake, frontendURL: "https://app.example.test"}
	router := gin.New()
	h.RegisterRoutes(router)
	return router
}

func TestGoogleCallbackPassesIntentQuery(t *testing.T) {
	cases := map[string]dto.RegistrationIntent{
		"/auth/google/callback?intent=social": dto.IntentSocial,
		"/auth/google/callback":               dto.IntentOperator,
	}
	for target, want := range cases {
		fake := &fakeGoogleAuthenticator{err: context.Canceled} // stop after the call; redirect is not under test
		router := newIntentRouter(fake)

		form := url.Values{"credential": {"tok"}, "g_csrf_token": {"c"}}
		req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "g_csrf_token", Value: "c"})
		router.ServeHTTP(httptest.NewRecorder(), req)

		if fake.calls != 1 || fake.req.Intent != want {
			t.Fatalf("%s: calls=%d intent=%q, want intent %q", target, fake.calls, fake.req.Intent, want)
		}
	}
}

func TestGoogleCallbackInvalidIntentRedirects(t *testing.T) {
	fake := &fakeGoogleAuthenticator{}
	router := newIntentRouter(fake)

	form := url.Values{"credential": {"tok"}, "g_csrf_token": {"c"}}
	req := httptest.NewRequest(http.MethodPost, "/auth/google/callback?intent=admin", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "g_csrf_token", Value: "c"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://app.example.test/auth/login?google_error=1" {
		t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if fake.calls != 0 {
		t.Fatal("service must not be called")
	}
}

func TestGoogleAuthRejectsUnknownIntent(t *testing.T) {
	fake := &fakeGoogleAuthenticator{}
	router := newIntentRouter(fake)

	req := httptest.NewRequest(http.MethodPost, "/auth/google", strings.NewReader(`{"id_token":"tok","intent":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if fake.calls != 0 {
		t.Fatal("service must not be called")
	}
}
