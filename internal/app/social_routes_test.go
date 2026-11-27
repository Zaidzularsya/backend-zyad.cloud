package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/social"
	socialhandler "zyad.cloud/internal/modules/social/handler"
	socialservice "zyad.cloud/internal/modules/social/service"
)

type fakeSocialAttribution struct {
	visits []socialservice.VisitInput
}

func (f *fakeSocialAttribution) IsKnownCode(_ context.Context, code string) (bool, error) {
	return code == "ABCD2345", nil
}

func (f *fakeSocialAttribution) RecordVisit(_ context.Context, in socialservice.VisitInput) error {
	f.visits = append(f.visits, in)
	return nil
}

func (f *fakeSocialAttribution) Summary(_ context.Context, userID string) (socialservice.Summary, error) {
	return socialservice.Summary{Code: "ABCD2345", ShareURL: "https://zyad.test/r/ABCD2345", Visits: 3, Signups: 1}, nil
}

type fakeSocialRelations struct{ gotUser string }

func (f *fakeSocialRelations) ForUser(_ context.Context, userID string) ([]socialservice.CustomerRelation, error) {
	f.gotUser = userID
	return []socialservice.CustomerRelation{{OrganizationID: "org-1", OrganizationName: "ISP", Source: "claim", CustomerLinkID: "link-1"}}, nil
}

type fakeSocialAuthenticator struct{}

func (fakeSocialAuthenticator) AuthenticateAccessToken(_ context.Context, token string) (middleware.AuthenticatedUser, error) {
	return middleware.AuthenticatedUser{ID: "user-" + token, SessionID: "session-" + token}, nil
}

// noOrganizationResolver: a social-only user has no workspace; /social/me must still work.
type noOrganizationResolver struct{}

func (noOrganizationResolver) ResolveAuthenticatedOrganization(context.Context, string, string, string, string) (coretenant.Context, bool, error) {
	return coretenant.Context{}, false, nil
}

func TestSocialRoutesAreMounted(t *testing.T) {
	attribution := &fakeSocialAttribution{}
	relations := &fakeSocialRelations{}
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	router, err := newRouter(Dependencies{
		Config:               config.Config{App: config.AppConfig{Env: "test"}},
		Authenticator:        fakeSocialAuthenticator{},
		OrganizationResolver: noOrganizationResolver{},
		SocialHandlers: social.Handlers{
			Public: socialhandler.NewPublicHandler(attribution, counter),
			Me:     socialhandler.NewMeHandler(attribution, relations),
		},
	})
	if err != nil {
		t.Fatalf("newRouter: %v", err)
	}
	do := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	if w := do(http.MethodGet, "/api/v1/public/referrals/ABCD2345", "", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"valid":true`) {
		t.Fatalf("known code: %d %s", w.Code, w.Body)
	}
	if w := do(http.MethodGet, "/api/v1/public/referrals/QQQQ2222", "", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"valid":false`) {
		t.Fatalf("unknown code: %d %s", w.Code, w.Body)
	}

	for i := 1; i <= 60; i++ {
		if w := do(http.MethodPost, "/api/v1/public/attribution/visits", `{"code":"ABCD2345","channel":"wa"}`, ""); w.Code != http.StatusNoContent {
			t.Fatalf("visit %d: %d %s", i, w.Code, w.Body)
		}
	}
	if w := do(http.MethodPost, "/api/v1/public/attribution/visits", `{"code":"ABCD2345"}`, ""); w.Code != http.StatusTooManyRequests {
		t.Fatalf("visit 61: %d, want 429", w.Code)
	}
	if len(attribution.visits) != 60 || attribution.visits[0].Channel != "wa" {
		t.Fatalf("visits = %d %+v", len(attribution.visits), attribution.visits[0])
	}
	for k := range counter.counts {
		if !strings.HasPrefix(k, "rl:attr:visit:ip:") {
			t.Fatalf("rate limit key %q", k)
		}
	}

	if w := do(http.MethodGet, "/api/v1/social/me/attribution", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous /social/me: %d, want 401", w.Code)
	}
	if w := do(http.MethodGet, "/api/v1/social/me/attribution", "", "1"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"share_url":"https://zyad.test/r/ABCD2345"`) {
		t.Fatalf("attribution: %d %s", w.Code, w.Body)
	}
	w := do(http.MethodGet, "/api/v1/social/me/customer-relations", "", "1")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"customer_link_id":"link-1"`) || relations.gotUser != "user-1" {
		t.Fatalf("customer relations: %d %s (user %q)", w.Code, w.Body, relations.gotUser)
	}
}
