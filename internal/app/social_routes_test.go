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
	socialdomain "zyad.cloud/internal/modules/social/domain"
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

type fakeSocialClaims struct {
	gotUser   socialservice.AuthUser
	revoked   string
	byStaff   bool
	acceptErr error
}

func (f *fakeSocialClaims) Offers(_ context.Context, user socialservice.AuthUser) ([]socialservice.ClaimOffer, error) {
	f.gotUser = user
	return []socialservice.ClaimOffer{{OrganizationID: "org-1", OrganizationName: "ISP", ContactID: "c-1", ContactName: "Budi", Kind: "email_match"}}, nil
}

func (f *fakeSocialClaims) Accept(_ context.Context, user socialservice.AuthUser, orgID, contactID string) (socialdomain.CustomerLink, error) {
	return socialdomain.CustomerLink{}, f.acceptErr
}

func (f *fakeSocialClaims) Decline(context.Context, socialservice.AuthUser, string, string) error {
	return nil
}

func (f *fakeSocialClaims) Revoke(_ context.Context, actor, linkID string, byStaff bool) error {
	f.revoked, f.byStaff = actor+"/"+linkID, byStaff
	return nil
}

// fakeSocialAuthenticator: tokens starting with "v" belong to users with a verified email.
type fakeSocialAuthenticator struct{}

func (fakeSocialAuthenticator) AuthenticateAccessToken(_ context.Context, token string) (middleware.AuthenticatedUser, error) {
	return middleware.AuthenticatedUser{
		ID: "user-" + token, SessionID: "session-" + token,
		Email: token + "@x.test", EmailVerified: strings.HasPrefix(token, "v"),
	}, nil
}

// noOrganizationResolver: a social-only user has no workspace; /social/me must still work.
type noOrganizationResolver struct{}

func (noOrganizationResolver) ResolveAuthenticatedOrganization(context.Context, string, string, string, string) (coretenant.Context, bool, error) {
	return coretenant.Context{}, false, nil
}

func TestSocialRoutesAreMounted(t *testing.T) {
	attribution := &fakeSocialAttribution{}
	relations := &fakeSocialRelations{}
	claims := &fakeSocialClaims{acceptErr: socialdomain.ErrCustomerLinkExists}
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	router, err := newRouter(Dependencies{
		Config:               config.Config{App: config.AppConfig{Env: "test"}},
		Authenticator:        fakeSocialAuthenticator{},
		OrganizationResolver: noOrganizationResolver{},
		SocialHandlers: social.Handlers{
			Public: socialhandler.NewPublicHandler(attribution, counter),
			Me:     socialhandler.NewMeHandler(attribution, relations, claims),
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

	const claimBody = `{"organization_id":"0b6c3f0e-7a4e-4f3a-9a51-1f2d3c4b5a69","contact_id":"5a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"}`
	for _, r := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/social/me/customer-claims", ""},
		{http.MethodPost, "/api/v1/social/me/customer-claims/accept", claimBody},
		{http.MethodPost, "/api/v1/social/me/customer-claims/decline", claimBody},
		{http.MethodDelete, "/api/v1/social/me/customer-links/link-1", ""},
	} {
		if w := do(r.method, r.path, r.body, "1"); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "EMAIL_NOT_VERIFIED") {
			t.Fatalf("unverified %s %s: %d %s", r.method, r.path, w.Code, w.Body)
		}
	}
	w = do(http.MethodGet, "/api/v1/social/me/customer-claims", "", "v1")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"kind":"email_match"`) || claims.gotUser.Email != "v1@x.test" || !claims.gotUser.EmailVerified {
		t.Fatalf("customer claims: %d %s (user %+v)", w.Code, w.Body, claims.gotUser)
	}
	if w := do(http.MethodPost, "/api/v1/social/me/customer-claims/accept", claimBody, "v1"); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "CUSTOMER_LINK_EXISTS") {
		t.Fatalf("accept conflict: %d %s", w.Code, w.Body)
	}
	if w := do(http.MethodPost, "/api/v1/social/me/customer-claims/accept", `{"organization_id":"x"}`, "v1"); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("accept invalid body: %d %s", w.Code, w.Body)
	}
	if w := do(http.MethodPost, "/api/v1/social/me/customer-claims/decline", claimBody, "v1"); w.Code != http.StatusNoContent {
		t.Fatalf("decline: %d %s", w.Code, w.Body)
	}
	if w := do(http.MethodDelete, "/api/v1/social/me/customer-links/link-1", "", "v1"); w.Code != http.StatusNoContent || claims.revoked != "user-v1/link-1" || claims.byStaff {
		t.Fatalf("revoke: %d %s (%q, byStaff %v)", w.Code, w.Body, claims.revoked, claims.byStaff)
	}
}
