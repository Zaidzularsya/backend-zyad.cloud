package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/subscription"
	whatsappmodule "zyad.cloud/internal/modules/whatsapp"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

const testOrgID = "3f2a9c1b-7d4e-4a11-9b2c-000000000001"

func testScope(t *testing.T, orgID string) coretenant.Scope {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     orgID,
		OrganizationSlug:   "org",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("tenant context: %v", err)
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	return scope
}

// fakeSessionRepo is an in-memory SessionRepository keyed by organization.
type fakeSessionRepo struct {
	sessions map[string]domain.Session
	seq      int
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: map[string]domain.Session{}}
}

func (r *fakeSessionRepo) visible(scope coretenant.Scope, id string) (domain.Session, bool) {
	s, ok := r.sessions[id]
	return s, ok && s.OrganizationID == scope.OrganizationID() && s.DeletedAt == nil
}

func (r *fakeSessionRepo) Create(_ context.Context, scope coretenant.Scope, p repository.CreateSessionParams) (domain.Session, error) {
	r.seq++
	id := "session-" + string(rune('0'+r.seq))
	if p.IsDefault {
		for key, s := range r.sessions {
			if s.OrganizationID == scope.OrganizationID() {
				s.IsDefault = false
				r.sessions[key] = s
			}
		}
	}
	s := domain.Session{
		ID: id, OrganizationID: scope.OrganizationID(), Name: p.Name, DisplayName: p.DisplayName,
		Status: domain.SessionStatusStopped, Engine: p.Engine, Purpose: p.Purpose,
		IsDefault: p.IsDefault, AutoCreateLead: p.AutoCreateLead,
	}
	r.sessions[id] = s
	return s, nil
}

func (r *fakeSessionRepo) GetByID(_ context.Context, scope coretenant.Scope, id string) (domain.Session, error) {
	if s, ok := r.visible(scope, id); ok {
		return s, nil
	}
	return domain.Session{}, pgx.ErrNoRows
}

func (r *fakeSessionRepo) List(_ context.Context, scope coretenant.Scope) ([]domain.Session, error) {
	out := []domain.Session{}
	for id := range r.sessions {
		if s, ok := r.visible(scope, id); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *fakeSessionRepo) CountActive(ctx context.Context, scope coretenant.Scope) (int64, error) {
	list, _ := r.List(ctx, scope)
	return int64(len(list)), nil
}

func (r *fakeSessionRepo) Update(_ context.Context, scope coretenant.Scope, id string, p repository.UpdateSessionParams) (domain.Session, error) {
	s, ok := r.visible(scope, id)
	if !ok {
		return domain.Session{}, pgx.ErrNoRows
	}
	if p.DisplayName != nil {
		s.DisplayName = *p.DisplayName
	}
	if p.Purpose != nil {
		s.Purpose = *p.Purpose
	}
	r.sessions[id] = s
	return s, nil
}

func (r *fakeSessionRepo) UpdateStatus(_ context.Context, scope coretenant.Scope, id string, p repository.UpdateSessionStatusParams) (domain.Session, error) {
	s, ok := r.visible(scope, id)
	if !ok {
		return domain.Session{}, pgx.ErrNoRows
	}
	s.Status = p.Status
	at := p.At
	s.LastStatusAt = &at
	if p.Phone != nil {
		s.Phone = *p.Phone
	}
	if p.PushName != nil {
		s.PushName = *p.PushName
	}
	r.sessions[id] = s
	return s, nil
}

func (r *fakeSessionRepo) SoftDelete(_ context.Context, scope coretenant.Scope, id string, _ string) error {
	s, ok := r.visible(scope, id)
	if !ok {
		return pgx.ErrNoRows
	}
	now := time.Now()
	s.DeletedAt = &now
	r.sessions[id] = s
	return nil
}

// fakeProvider records calls and returns configured results.
type fakeProvider struct {
	created      []platformwhatsapp.CreateSessionRequest
	createErr    error
	info         platformwhatsapp.SessionInfo
	getErr       error
	deleteErr    error
	logoutErr    error
	deleted      []string
	pairingPhone string
	calls        int
}

func (p *fakeProvider) CreateSession(_ context.Context, r platformwhatsapp.CreateSessionRequest) (platformwhatsapp.SessionInfo, error) {
	p.calls++
	p.created = append(p.created, r)
	if p.createErr != nil {
		return platformwhatsapp.SessionInfo{}, p.createErr
	}
	return platformwhatsapp.SessionInfo{Name: r.Name, Status: platformwhatsapp.SessionStatusStarting}, nil
}

func (p *fakeProvider) GetSession(_ context.Context, name string) (platformwhatsapp.SessionInfo, error) {
	p.calls++
	if p.getErr != nil {
		return platformwhatsapp.SessionInfo{}, p.getErr
	}
	info := p.info
	info.Name = name
	return info, nil
}

func (p *fakeProvider) StartSession(_ context.Context, name string) (platformwhatsapp.SessionInfo, error) {
	p.calls++
	return platformwhatsapp.SessionInfo{Name: name, Status: platformwhatsapp.SessionStatusStarting}, nil
}

func (p *fakeProvider) StopSession(_ context.Context, name string) (platformwhatsapp.SessionInfo, error) {
	p.calls++
	return platformwhatsapp.SessionInfo{Name: name, Status: platformwhatsapp.SessionStatusStopped}, nil
}

func (p *fakeProvider) LogoutSession(_ context.Context, name string) (platformwhatsapp.SessionInfo, error) {
	p.calls++
	return platformwhatsapp.SessionInfo{Name: name, Status: platformwhatsapp.SessionStatusStopped}, p.logoutErr
}

func (p *fakeProvider) DeleteSession(_ context.Context, name string) error {
	p.calls++
	if p.deleteErr != nil {
		return p.deleteErr
	}
	p.deleted = append(p.deleted, name)
	return nil
}

func (p *fakeProvider) GetQR(_ context.Context, _ string) (platformwhatsapp.QRCode, error) {
	p.calls++
	return platformwhatsapp.QRCode{MimeType: "image/png", Data: "QUJD"}, nil
}

func (p *fakeProvider) RequestPairingCode(_ context.Context, _ string, phone string) (string, error) {
	p.calls++
	p.pairingPhone = phone
	return "ABCD-EFGH", nil
}

type fakeQuota struct {
	err       error
	used      int64
	featureOK string
}

func (q *fakeQuota) RequireQuotaValue(_ context.Context, _ string, featureKey, _ string, used, _ int64) error {
	q.used = used
	q.featureOK = featureKey
	return q.err
}

var testConfig = SessionServiceConfig{
	WebhookURL:     "https://api.example.test/api/v1/webhooks/waha",
	WebhookHMACKey: "hook-secret",
	Engine:         "GOWS",
}

func newTestService(repo *fakeSessionRepo, provider SessionProvider, quota SessionQuotaGuard) *SessionService {
	options := []SessionServiceOption{}
	if quota != nil {
		options = append(options, WithSessionQuotaGuard(quota))
	}
	return NewSessionService(repo, provider, testConfig, options...)
}

func appErrorCode(err error) string {
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

func TestCreateSessionRegistersOnProvider(t *testing.T) {
	repo, provider := newFakeSessionRepo(), &fakeProvider{}
	svc := newTestService(repo, provider, &fakeQuota{})
	scope := testScope(t, testOrgID)

	session, err := svc.Create(context.Background(), scope, CreateSessionInput{DisplayName: " Sales ", ActorUserID: "user-1"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !regexp.MustCompile(`^zc_3f2a9c1b7d4e_[a-z0-9]{6}$`).MatchString(session.Name) {
		t.Fatalf("session name = %q", session.Name)
	}
	if session.Purpose != domain.SessionPurposeSales || !session.IsDefault || session.DisplayName != "Sales" {
		t.Fatalf("session = %+v (first session should be default sales)", session)
	}
	if session.Status != domain.SessionStatusStarting || session.LastStatusAt == nil {
		t.Fatalf("status not applied from provider: %+v", session)
	}

	if len(provider.created) != 1 {
		t.Fatalf("provider create calls = %d", len(provider.created))
	}
	request := provider.created[0]
	if !request.Start || request.Name != session.Name {
		t.Fatalf("provider request = %+v", request)
	}
	webhook := request.Webhooks[0]
	if webhook.URL != testConfig.WebhookURL || webhook.HMACKey != "hook-secret" || len(webhook.Events) != 4 {
		t.Fatalf("webhook = %+v", webhook)
	}
	if request.Metadata["zyad.organization_id"] != testOrgID || request.Metadata["zyad.session_id"] != session.ID {
		t.Fatalf("metadata = %+v", request.Metadata)
	}
}

func TestCreateSessionQuotaExceededSkipsProvider(t *testing.T) {
	repo, provider := newFakeSessionRepo(), &fakeProvider{}
	svc := newTestService(repo, provider, &fakeQuota{err: subscription.QuotaExceededError()})

	_, err := svc.Create(context.Background(), testScope(t, testOrgID), CreateSessionInput{})
	if appErrorCode(err) != subscription.ErrCodeQuotaExceeded {
		t.Fatalf("error = %v, want quota exceeded", err)
	}
	if provider.calls != 0 || len(repo.sessions) != 0 {
		t.Fatalf("provider calls = %d, sessions = %d; want none", provider.calls, len(repo.sessions))
	}
}

func TestCreateSessionFallbackLimitWithoutEntitlementRow(t *testing.T) {
	repo, provider := newFakeSessionRepo(), &fakeProvider{}
	quota := &fakeQuota{err: subscription.FeatureNotEnabledError()}
	svc := newTestService(repo, provider, quota)
	scope := testScope(t, testOrgID)

	if _, err := svc.Create(context.Background(), scope, CreateSessionInput{}); err != nil {
		t.Fatalf("first Create() error = %v, want fallback limit 1 to allow it", err)
	}
	if quota.featureOK != domain.FeatureWhatsAppMaxSessions {
		t.Fatalf("quota feature = %q", quota.featureOK)
	}
	_, err := svc.Create(context.Background(), scope, CreateSessionInput{})
	if appErrorCode(err) != subscription.ErrCodeQuotaExceeded {
		t.Fatalf("second Create() error = %v, want quota exceeded", err)
	}
}

func TestCreateSessionProviderFailureRollsBack(t *testing.T) {
	repo := newFakeSessionRepo()
	provider := &fakeProvider{createErr: &platformwhatsapp.APIError{Operation: "create session", StatusCode: 500, Body: "boom"}}
	svc := newTestService(repo, provider, nil)
	scope := testScope(t, testOrgID)

	_, err := svc.Create(context.Background(), scope, CreateSessionInput{})
	if appErrorCode(err) != whatsappmodule.ErrProviderUnavailable.Code {
		t.Fatalf("error = %v, want provider error", err)
	}
	if total, _ := repo.CountActive(context.Background(), scope); total != 0 {
		t.Fatalf("active sessions after rollback = %d, want 0", total)
	}
}

func TestCreateSessionRequiresConfiguration(t *testing.T) {
	scope := testScope(t, testOrgID)

	repo := newFakeSessionRepo()
	svc := NewSessionService(repo, nil, testConfig)
	if _, err := svc.Create(context.Background(), scope, CreateSessionInput{}); !errors.Is(err, whatsappmodule.ErrNotConfigured) {
		t.Fatalf("nil provider error = %v", err)
	}

	svc = NewSessionService(repo, &fakeProvider{}, SessionServiceConfig{WebhookURL: testConfig.WebhookURL})
	if _, err := svc.Create(context.Background(), scope, CreateSessionInput{}); !errors.Is(err, whatsappmodule.ErrNotConfigured) {
		t.Fatalf("missing hmac key error = %v", err)
	}
	if len(repo.sessions) != 0 {
		t.Fatal("session stored although provider is not configured")
	}
}

func TestCreateSessionRejectsInvalidPurpose(t *testing.T) {
	svc := newTestService(newFakeSessionRepo(), &fakeProvider{}, nil)
	_, err := svc.Create(context.Background(), testScope(t, testOrgID), CreateSessionInput{Purpose: "broadcast"})
	if !errors.Is(err, whatsappmodule.ErrInvalidPurpose) {
		t.Fatalf("error = %v", err)
	}
}

func TestSessionIsolationBetweenOrganizations(t *testing.T) {
	repo := newFakeSessionRepo()
	svc := newTestService(repo, &fakeProvider{}, nil)
	a := testScope(t, testOrgID)
	b := testScope(t, "3f2a9c1b-7d4e-4a11-9b2c-000000000002")

	session, err := svc.Create(context.Background(), a, CreateSessionInput{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.Get(context.Background(), b, session.ID); !errors.Is(err, whatsappmodule.ErrSessionNotFound) {
		t.Fatalf("B Get(A) error = %v, want not found", err)
	}
	if err := svc.Delete(context.Background(), b, session.ID, ""); !errors.Is(err, whatsappmodule.ErrSessionNotFound) {
		t.Fatalf("B Delete(A) error = %v, want not found", err)
	}
}

func createdSession(t *testing.T, svc *SessionService, scope coretenant.Scope) domain.Session {
	t.Helper()
	session, err := svc.Create(context.Background(), scope, CreateSessionInput{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return session
}

func TestQRRequiresScanningStatus(t *testing.T) {
	provider := &fakeProvider{info: platformwhatsapp.SessionInfo{Status: platformwhatsapp.SessionStatusWorking}}
	svc := newTestService(newFakeSessionRepo(), provider, nil)
	scope := testScope(t, testOrgID)
	session := createdSession(t, svc, scope)

	if _, err := svc.QR(context.Background(), scope, session.ID); !errors.Is(err, whatsappmodule.ErrSessionNotScanning) {
		t.Fatalf("QR() on WORKING error = %v", err)
	}

	provider.info.Status = platformwhatsapp.SessionStatusScanQRCode
	qr, err := svc.QR(context.Background(), scope, session.ID)
	if err != nil {
		t.Fatalf("QR() error = %v", err)
	}
	if qr.DataURL != "data:image/png;base64,QUJD" || qr.Status != domain.SessionStatusScanQRCode {
		t.Fatalf("qr = %+v", qr)
	}
}

func TestPairingCode(t *testing.T) {
	provider := &fakeProvider{info: platformwhatsapp.SessionInfo{Status: platformwhatsapp.SessionStatusScanQRCode}}
	svc := newTestService(newFakeSessionRepo(), provider, nil)
	scope := testScope(t, testOrgID)
	session := createdSession(t, svc, scope)

	if _, err := svc.PairingCode(context.Background(), scope, session.ID, "12"); !errors.Is(err, whatsappmodule.ErrInvalidPairingPhone) {
		t.Fatalf("invalid phone error = %v", err)
	}
	code, err := svc.PairingCode(context.Background(), scope, session.ID, "0812-3456-7890")
	if err != nil || code != "ABCD-EFGH" {
		t.Fatalf("PairingCode() = %q, %v", code, err)
	}
	if provider.pairingPhone != "6281234567890" {
		t.Fatalf("phone sent to provider = %q", provider.pairingPhone)
	}
}

func TestStatusSyncStoresLinkedNumber(t *testing.T) {
	provider := &fakeProvider{info: platformwhatsapp.SessionInfo{
		Status: platformwhatsapp.SessionStatusWorking,
		Me:     &platformwhatsapp.Me{ID: "6281234567890@c.us", PushName: "Toko A"},
	}}
	repo := newFakeSessionRepo()
	svc := newTestService(repo, provider, nil)
	scope := testScope(t, testOrgID)
	session := createdSession(t, svc, scope)

	// Freshly created: status is not stale, so no provider call.
	calls := provider.calls
	if _, err := svc.Status(context.Background(), scope, session.ID); err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if provider.calls != calls {
		t.Fatal("Status() called provider although stored status is fresh")
	}

	svc.now = func() time.Time { return time.Now().UTC().Add(time.Minute) }
	updated, err := svc.Status(context.Background(), scope, session.ID)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if updated.Status != domain.SessionStatusWorking || updated.Phone != "6281234567890" || updated.PushName != "Toko A" {
		t.Fatalf("updated = %+v", updated)
	}

	provider.getErr = platformwhatsapp.ErrTimeout
	svc.now = func() time.Time { return time.Now().UTC().Add(time.Hour) }
	fallback, err := svc.Status(context.Background(), scope, session.ID)
	if err != nil || fallback.Status != domain.SessionStatusWorking {
		t.Fatalf("Status() on provider failure = %+v, %v; want stored status", fallback, err)
	}
}

func TestDeleteSession(t *testing.T) {
	provider := &fakeProvider{logoutErr: &platformwhatsapp.APIError{StatusCode: 422, Body: "not logged in"}}
	repo := newFakeSessionRepo()
	svc := newTestService(repo, provider, nil)
	scope := testScope(t, testOrgID)
	session := createdSession(t, svc, scope)

	if err := svc.Delete(context.Background(), scope, session.ID, "user-1"); err != nil {
		t.Fatalf("Delete() error = %v (logout failure must not block delete)", err)
	}
	if len(provider.deleted) != 1 || provider.deleted[0] != session.Name {
		t.Fatalf("provider deleted = %v", provider.deleted)
	}
	if _, err := svc.Get(context.Background(), scope, session.ID); !errors.Is(err, whatsappmodule.ErrSessionNotFound) {
		t.Fatalf("Get after delete error = %v", err)
	}
}

func TestDeleteSessionKeepsLocalRowWhenProviderFails(t *testing.T) {
	provider := &fakeProvider{}
	repo := newFakeSessionRepo()
	svc := newTestService(repo, provider, nil)
	scope := testScope(t, testOrgID)
	session := createdSession(t, svc, scope)

	provider.deleteErr = platformwhatsapp.ErrTimeout
	if err := svc.Delete(context.Background(), scope, session.ID, ""); appErrorCode(err) != whatsappmodule.ErrProviderUnavailable.Code {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Get(context.Background(), scope, session.ID); err != nil {
		t.Fatalf("local session removed although provider delete failed: %v", err)
	}
}

func TestProviderActionsMapRemoteMissing(t *testing.T) {
	provider := &fakeProvider{}
	svc := newTestService(newFakeSessionRepo(), provider, nil)
	scope := testScope(t, testOrgID)
	session := createdSession(t, svc, scope)

	provider.getErr = &platformwhatsapp.APIError{Operation: "get session", StatusCode: 404}
	if _, err := svc.Reconcile(context.Background(), scope, session.ID); !errors.Is(err, platformwhatsapp.ErrSessionNotFound) {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if _, err := svc.QR(context.Background(), scope, session.ID); appErrorCode(err) != whatsappmodule.ErrRemoteSessionMissing.Code {
		t.Fatalf("QR() error = %v, want remote session missing", err)
	}
}
