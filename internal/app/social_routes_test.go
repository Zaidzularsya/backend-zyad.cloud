package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
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

type fakeSocialStaffLinks struct {
	requested []string
	revoked   []string
	err       error
}

func (f *fakeSocialStaffLinks) Request(_ context.Context, scope coretenant.Scope, actor, contactID, email string) (socialdomain.CustomerLink, error) {
	if f.err != nil {
		return socialdomain.CustomerLink{}, f.err
	}
	f.requested = append(f.requested, scope.OrganizationID()+"/"+actor+"/"+contactID+"/"+email)
	return socialdomain.CustomerLink{ID: "link-1", UserID: "user-1", ContactID: contactID, Status: socialdomain.LinkStatusPendingUser, VerifiedVia: socialdomain.VerifiedViaStaff}, nil
}

func (f *fakeSocialStaffLinks) ListForContact(_ context.Context, scope coretenant.Scope, contactID string) ([]socialdomain.CustomerLink, error) {
	return []socialdomain.CustomerLink{{ID: "link-1", UserID: "user-1", ContactID: contactID, Status: socialdomain.LinkStatusActive, VerifiedVia: socialdomain.VerifiedViaStaff}}, nil
}

func (f *fakeSocialStaffLinks) Revoke(_ context.Context, scope coretenant.Scope, actor, linkID string) error {
	if f.err != nil {
		return f.err
	}
	f.revoked = append(f.revoked, scope.OrganizationID()+"/"+actor+"/"+linkID)
	return nil
}

type fakeSocialInvites struct {
	gotUser  socialservice.AuthUser
	gotToken string
	sent     []string
}

func (f *fakeSocialInvites) Accept(_ context.Context, user socialservice.AuthUser, token string) (socialdomain.CustomerLink, error) {
	f.gotUser, f.gotToken = user, token
	if token != "good-token" {
		return socialdomain.CustomerLink{}, socialdomain.ErrInviteInvalid
	}
	return socialdomain.CustomerLink{ID: "link-9", Status: socialdomain.LinkStatusActive, VerifiedVia: socialdomain.VerifiedViaInvite}, nil
}

func (f *fakeSocialInvites) Send(_ context.Context, scope coretenant.Scope, actor, contactID string) error {
	f.sent = append(f.sent, scope.OrganizationID()+"/"+actor+"/"+contactID)
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
	invites := &fakeSocialInvites{}
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	router, err := newRouter(Dependencies{
		Config:               config.Config{App: config.AppConfig{Env: "test"}},
		Authenticator:        fakeSocialAuthenticator{},
		OrganizationResolver: noOrganizationResolver{},
		SocialHandlers: social.Handlers{
			Public: socialhandler.NewPublicHandler(attribution, counter),
			Me:     socialhandler.NewMeHandler(attribution, relations, claims, invites, counter),
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

	const linkID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	const claimBody = `{"organization_id":"0b6c3f0e-7a4e-4f3a-9a51-1f2d3c4b5a69","contact_id":"5a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"}`
	for _, r := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/social/me/customer-claims", ""},
		{http.MethodPost, "/api/v1/social/me/customer-claims/accept", claimBody},
		{http.MethodPost, "/api/v1/social/me/customer-claims/decline", claimBody},
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
	if w := do(http.MethodDelete, "/api/v1/social/me/customer-links/"+linkID, "", "v1"); w.Code != http.StatusNoContent || claims.revoked != "user-v1/"+linkID || claims.byStaff {
		t.Fatalf("revoke: %d %s (%q, byStaff %v)", w.Code, w.Body, claims.revoked, claims.byStaff)
	}

	if w := do(http.MethodDelete, "/api/v1/social/me/customer-links/not-a-uuid", "", "v1"); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "CUSTOMER_LINK_NOT_FOUND") {
		t.Fatalf("revoke malformed id: %d %s", w.Code, w.Body)
	}
	// A user can always undo their own link, verified email or not.
	if w := do(http.MethodDelete, "/api/v1/social/me/customer-links/"+linkID, "", "1"); w.Code != http.StatusNoContent || claims.revoked != "user-1/"+linkID {
		t.Fatalf("revoke unverified: %d %s (%q)", w.Code, w.Body, claims.revoked)
	}

	// Invite acceptance is open to unverified accounts: the token proves the email.
	w = do(http.MethodPost, "/api/v1/social/me/customer-invites/accept", `{"token":"good-token"}`, "1")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"verified_via":"invite"`) || invites.gotUser.EmailVerified || invites.gotUser.ID != "user-1" {
		t.Fatalf("accept invite unverified: %d %s (user %+v)", w.Code, w.Body, invites.gotUser)
	}
	if w := do(http.MethodPost, "/api/v1/social/me/customer-invites/accept", `{"token":"bad"}`, "1"); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "CLAIM_INVITE_INVALID") {
		t.Fatalf("accept invalid invite: %d %s", w.Code, w.Body)
	}
	if w := do(http.MethodPost, "/api/v1/social/me/customer-invites/accept", `{}`, "1"); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("accept invite without token: %d %s", w.Code, w.Body)
	}
	if w := do(http.MethodPost, "/api/v1/social/me/customer-invites/accept", `{"token":"good-token"}`, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous accept invite: %d", w.Code)
	}

	// /social/me writes share one per-user bucket of 20 per minute; a fresh
	// user ("v2") mixing write routes is cut off at the 21st.
	writes := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/social/me/customer-claims/decline", claimBody},
		{http.MethodDelete, "/api/v1/social/me/customer-links/" + linkID, ""},
		{http.MethodPost, "/api/v1/social/me/customer-invites/accept", `{"token":"good-token"}`},
		{http.MethodPost, "/api/v1/social/me/customer-claims/accept", claimBody},
	}
	for i := 1; i <= 20; i++ {
		r := writes[i%len(writes)]
		if w := do(r.method, r.path, r.body, "v2"); w.Code == http.StatusTooManyRequests {
			t.Fatalf("me write %d (%s %s): 429 too early", i, r.method, r.path)
		}
	}
	if w := do(http.MethodPost, "/api/v1/social/me/customer-claims/decline", claimBody, "v2"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("me write 21: %d, want 429", w.Code)
	}
	if w := do(http.MethodGet, "/api/v1/social/me/customer-claims", "", "v2"); w.Code != http.StatusOK {
		t.Fatalf("reads are not rate limited: %d", w.Code)
	}
	var meKeys int
	for k := range counter.counts {
		if strings.HasPrefix(k, "rl:social:me:user:") {
			meKeys++
			if !strings.HasPrefix(k, "rl:social:me:user:user-1:") && !strings.HasPrefix(k, "rl:social:me:user:user-v1:") && !strings.HasPrefix(k, "rl:social:me:user:user-v2:") {
				t.Fatalf("me rate limit key %q", k)
			}
		} else if !strings.HasPrefix(k, "rl:attr:visit:ip:") {
			t.Fatalf("unexpected rate limit key %q", k)
		}
	}
	if meKeys == 0 {
		t.Fatal("no per-user rate limit keys recorded")
	}
}

func TestSocialCustomerInviteRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "0b6c3f0e-7a4e-4f3a-9a51-1f2d3c4b5a69", OrganizationSlug: "isp",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource: coretenant.ResolutionSourceInternal, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	invites := &fakeSocialInvites{}
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	engine := gin.New()
	group := engine.Group("/app/social", func(c *gin.Context) {
		c.Set(permissionmiddleware.UserIDContextKey, "staff-1")
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
	})
	socialhandler.NewTenantHandler(nil, invites, nil, counter).RegisterRoutes(group, nil)
	do := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/app/social/customer-invites", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	if w := do(`{"contact_id":"not-a-uuid"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("malformed contact_id: %d %s", w.Code, w.Body)
	}
	const body = `{"contact_id":"5a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"}`
	if w := do(body); w.Code != http.StatusNoContent {
		t.Fatalf("send: %d %s", w.Code, w.Body)
	}
	if want := tc.OrganizationID() + "/staff-1/5a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"; len(invites.sent) != 1 || invites.sent[0] != want {
		t.Fatalf("sent = %v, want [%s]", invites.sent, want)
	}
	for i := 3; i <= 30; i++ {
		if w := do(body); w.Code != http.StatusNoContent {
			t.Fatalf("send %d: %d", i, w.Code)
		}
	}
	if w := do(body); w.Code != http.StatusTooManyRequests {
		t.Fatalf("send 31: %d, want 429", w.Code)
	}
	for k := range counter.counts {
		if !strings.HasPrefix(k, "rl:social:invite:org:"+tc.OrganizationID()+":") {
			t.Fatalf("rate limit key %q", k)
		}
	}
}

func TestSocialStaffCustomerLinkRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "0b6c3f0e-7a4e-4f3a-9a51-1f2d3c4b5a69", OrganizationSlug: "isp",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource: coretenant.ResolutionSourceInternal, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	staff := &fakeSocialStaffLinks{}
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	engine := gin.New()
	group := engine.Group("/app/social", func(c *gin.Context) {
		c.Set(permissionmiddleware.UserIDContextKey, "staff-1")
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
	})
	socialhandler.NewTenantHandler(nil, nil, staff, counter).RegisterRoutes(group, nil)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	const contactID = "5a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	const linkID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"

	for _, body := range []string{`{"contact_id":"x","user_email":"a@x.test"}`, `{"contact_id":"` + contactID + `","user_email":"nope"}`, `{"contact_id":"` + contactID + `"}`} {
		if w := do(http.MethodPost, "/app/social/customer-links", body); w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid body %s: %d %s", body, w.Code, w.Body)
		}
	}
	w := do(http.MethodPost, "/app/social/customer-links", `{"contact_id":"`+contactID+`","user_email":"a@x.test"}`)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"status":"pending_user"`) ||
		len(staff.requested) != 1 || staff.requested[0] != tc.OrganizationID()+"/staff-1/"+contactID+"/a@x.test" {
		t.Fatalf("request: %d %s (%v)", w.Code, w.Body, staff.requested)
	}

	if w := do(http.MethodGet, "/app/social/customer-links?contact_id=bad", ""); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("list malformed contact_id: %d %s", w.Code, w.Body)
	}
	if w := do(http.MethodGet, "/app/social/customer-links", ""); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("list missing contact_id: %d %s", w.Code, w.Body)
	}
	if w := do(http.MethodGet, "/app/social/customer-links?contact_id="+contactID, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"link-1"`) {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}

	if w := do(http.MethodDelete, "/app/social/customer-links/not-a-uuid", ""); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "CUSTOMER_LINK_NOT_FOUND") {
		t.Fatalf("revoke malformed id: %d %s", w.Code, w.Body)
	}
	if w := do(http.MethodDelete, "/app/social/customer-links/"+linkID, ""); w.Code != http.StatusNoContent ||
		len(staff.revoked) != 1 || staff.revoked[0] != tc.OrganizationID()+"/staff-1/"+linkID {
		t.Fatalf("revoke: %d %s (%v)", w.Code, w.Body, staff.revoked)
	}

	staff.err = socialdomain.ErrUserNotFound
	if w := do(http.MethodPost, "/app/social/customer-links", `{"contact_id":"`+contactID+`","user_email":"a@x.test"}`); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "USER_NOT_FOUND") {
		t.Fatalf("unknown user: %d %s", w.Code, w.Body)
	}
	staff.err = nil

	// POST customer-links: 60 per organization per hour. 5 requests above
	// (3 invalid bodies, 1 success, 1 unknown user) already count.
	for i := 6; i <= 60; i++ {
		if w := do(http.MethodPost, "/app/social/customer-links", `{"contact_id":"`+contactID+`","user_email":"a@x.test"}`); w.Code != http.StatusCreated {
			t.Fatalf("request %d: %d %s", i, w.Code, w.Body)
		}
	}
	if w := do(http.MethodPost, "/app/social/customer-links", `{"contact_id":"`+contactID+`","user_email":"a@x.test"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("request 61: %d, want 429", w.Code)
	}
	// Unlinking has its own bucket, so it still works.
	if w := do(http.MethodDelete, "/app/social/customer-links/"+linkID, ""); w.Code != http.StatusNoContent {
		t.Fatalf("revoke after link limit: %d %s", w.Code, w.Body)
	}
	org := tc.OrganizationID()
	var linkKeys, unlinkKeys int
	for k := range counter.counts {
		switch {
		case strings.HasPrefix(k, "rl:social:link:org:"+org+":"):
			linkKeys++
		case strings.HasPrefix(k, "rl:social:unlink:org:"+org+":"):
			unlinkKeys++
		default:
			t.Fatalf("rate limit key %q", k)
		}
	}
	if linkKeys == 0 || unlinkKeys == 0 {
		t.Fatalf("link keys %d, unlink keys %d", linkKeys, unlinkKeys)
	}
}

func TestSocialTenantSettingsRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "0b6c3f0e-7a4e-4f3a-9a51-1f2d3c4b5a69", OrganizationSlug: "isp",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource: coretenant.ResolutionSourceInternal, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	engine := gin.New()
	group := engine.Group("/app/social", func(c *gin.Context) {
		c.Set(permissionmiddleware.UserIDContextKey, "staff-1")
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
	})
	socialhandler.NewTenantHandler(fakeSocialSettings{}, nil, nil, counter).RegisterRoutes(group, nil)
	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/app/social/settings", strings.NewReader(`{"customer_claim_auto_offer":false}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	for i := 1; i <= 30; i++ {
		if w := do(); w.Code != http.StatusOK {
			t.Fatalf("update %d: %d %s", i, w.Code, w.Body)
		}
	}
	if w := do(); w.Code != http.StatusTooManyRequests {
		t.Fatalf("update 31: %d, want 429", w.Code)
	}
	for k := range counter.counts {
		if !strings.HasPrefix(k, "rl:social:settings:org:"+tc.OrganizationID()+":") {
			t.Fatalf("rate limit key %q", k)
		}
	}
}

type fakeSocialSettings struct{}

func (fakeSocialSettings) Get(context.Context, string) (socialdomain.TenantSettings, error) {
	return socialdomain.TenantSettings{CustomerClaimAutoOffer: true}, nil
}

func (fakeSocialSettings) SetAutoOffer(_ context.Context, _ string, autoOffer bool, _ string) (socialdomain.TenantSettings, error) {
	return socialdomain.TenantSettings{CustomerClaimAutoOffer: autoOffer}, nil
}
