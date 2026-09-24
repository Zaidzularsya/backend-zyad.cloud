package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/subscription"
	whatsappmodule "zyad.cloud/internal/modules/whatsapp"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
	"zyad.cloud/internal/shared/phone"
)

// WebhookEvents are the WAHA events every app-created session subscribes to.
var WebhookEvents = []string{
	domain.EventSessionStatus,
	domain.EventMessage,
	domain.EventMessageAny,
	domain.EventMessageAck,
}

const (
	defaultStatusStaleAfter = 15 * time.Second
	sessionNamePrefix       = "zc_"
	sessionNameRandomLength = 6
	createNameAttempts      = 3
)

// SessionProvider is the subset of the WAHA client the session service uses.
type SessionProvider interface {
	CreateSession(ctx context.Context, request platformwhatsapp.CreateSessionRequest) (platformwhatsapp.SessionInfo, error)
	GetSession(ctx context.Context, name string) (platformwhatsapp.SessionInfo, error)
	StartSession(ctx context.Context, name string) (platformwhatsapp.SessionInfo, error)
	StopSession(ctx context.Context, name string) (platformwhatsapp.SessionInfo, error)
	LogoutSession(ctx context.Context, name string) (platformwhatsapp.SessionInfo, error)
	DeleteSession(ctx context.Context, name string) error
	GetQR(ctx context.Context, name string) (platformwhatsapp.QRCode, error)
	RequestPairingCode(ctx context.Context, name, phone string) (string, error)
}

// SessionQuotaGuard is a narrow view of SubscriptionGuardService (pattern:
// crm/service/contact_service.go ContactQuotaGuard).
type SessionQuotaGuard interface {
	RequireQuotaValue(ctx context.Context, organizationID, featureKey, limitKey string, usedValue, delta int64) error
}

type SessionServiceConfig struct {
	// WebhookURL is the public URL WAHA posts events to
	// (WHATSAPP_API_URL_CALLBACK, e.g. https://api.zyad.online/api/v1/webhooks/waha).
	WebhookURL     string
	WebhookHMACKey string
	Engine         string
	// StatusStaleAfter controls when Status re-reads WAHA instead of
	// returning the stored status.
	StatusStaleAfter time.Duration
}

type CreateSessionInput struct {
	DisplayName    string
	Purpose        domain.SessionPurpose
	IsDefault      bool
	AutoCreateLead bool
	ActorUserID    string
}

type UpdateSessionInput struct {
	DisplayName    *string
	IsDefault      *bool
	Purpose        *domain.SessionPurpose
	AutoCreateLead *bool
	ActorUserID    string
}

type SessionQR struct {
	DataURL string
	Status  domain.SessionStatus
}

type SessionService struct {
	repo     repository.SessionRepository
	provider SessionProvider
	quota    SessionQuotaGuard
	config   SessionServiceConfig
	log      *slog.Logger
	now      func() time.Time
}

type SessionServiceOption func(*SessionService)

func WithSessionQuotaGuard(guard SessionQuotaGuard) SessionServiceOption {
	return func(s *SessionService) { s.quota = guard }
}

func WithSessionLogger(log *slog.Logger) SessionServiceOption {
	return func(s *SessionService) { s.log = log }
}

// NewSessionService builds the service. provider may be nil when WAHA is not
// configured: reads keep working, provider actions return ErrNotConfigured.
func NewSessionService(repo repository.SessionRepository, provider SessionProvider, config SessionServiceConfig, options ...SessionServiceOption) *SessionService {
	if config.StatusStaleAfter <= 0 {
		config.StatusStaleAfter = defaultStatusStaleAfter
	}
	s := &SessionService{
		repo:     repo,
		provider: provider,
		config:   config,
		log:      slog.Default(),
		now:      func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *SessionService) List(ctx context.Context, scope coretenant.Scope) ([]domain.Session, error) {
	return s.repo.List(ctx, scope)
}

func (s *SessionService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error) {
	session, err := s.repo.GetByID(ctx, scope, id)
	return session, whatsappmodule.MapSessionNotFound(err)
}

// Create registers the session locally (with its directory entry, so early
// webhooks can already be resolved), then creates and starts it on WAHA. If
// WAHA fails, the local row is soft-deleted again.
func (s *SessionService) Create(ctx context.Context, scope coretenant.Scope, input CreateSessionInput) (domain.Session, error) {
	if input.Purpose == "" {
		input.Purpose = domain.SessionPurposeSales
	}
	if !input.Purpose.IsValid() {
		return domain.Session{}, whatsappmodule.ErrInvalidPurpose
	}
	if !s.canCreateRemote() {
		return domain.Session{}, whatsappmodule.ErrNotConfigured
	}

	existing, err := s.repo.CountActive(ctx, scope)
	if err != nil {
		return domain.Session{}, err
	}
	if err := s.requireSessionQuota(ctx, scope, existing); err != nil {
		return domain.Session{}, err
	}

	session, err := s.createLocal(ctx, scope, input, existing == 0)
	if err != nil {
		return domain.Session{}, err
	}

	info, err := s.provider.CreateSession(ctx, platformwhatsapp.CreateSessionRequest{
		Name:  session.Name,
		Start: true,
		Metadata: map[string]string{
			"zyad.organization_id": scope.OrganizationID(),
			"zyad.session_id":      session.ID,
		},
		Webhooks: []platformwhatsapp.WebhookConfig{{
			URL:     s.config.WebhookURL,
			Events:  WebhookEvents,
			HMACKey: s.config.WebhookHMACKey,
		}},
	})
	if err != nil {
		if deleteErr := s.repo.SoftDelete(ctx, scope, session.ID, input.ActorUserID); deleteErr != nil {
			s.log.Error("whatsapp: rollback local session after provider failure", "session_id", session.ID, "error", deleteErr)
		}
		s.log.Warn("whatsapp: create session on provider failed", "session_id", session.ID, "error", err)
		return domain.Session{}, whatsappmodule.MapProviderError(err)
	}

	return s.applyProviderInfo(ctx, scope, session, info)
}

// requireSessionQuota enforces whatsapp.max_sessions. Organizations whose plan
// was synced before the feature existed have no runtime entitlement row;
// they get DefaultMaxSessionsFallback until the next plan sync.
func (s *SessionService) requireSessionQuota(ctx context.Context, scope coretenant.Scope, existing int64) error {
	if s.quota == nil {
		return nil
	}
	err := s.quota.RequireQuotaValue(ctx, scope.OrganizationID(), domain.FeatureWhatsAppMaxSessions, "limit", existing, 1)
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) && appErr.Code == subscription.ErrCodeFeatureNotEnabled {
		if existing+1 > domain.DefaultMaxSessionsFallback {
			return subscription.QuotaExceededError()
		}
		return nil
	}
	return err
}

func (s *SessionService) createLocal(ctx context.Context, scope coretenant.Scope, input CreateSessionInput, first bool) (domain.Session, error) {
	var lastErr error
	for attempt := 0; attempt < createNameAttempts; attempt++ {
		name, err := generateSessionName(scope.OrganizationID())
		if err != nil {
			return domain.Session{}, err
		}
		session, err := s.repo.Create(ctx, scope, repository.CreateSessionParams{
			Name:           name,
			DisplayName:    strings.TrimSpace(input.DisplayName),
			Engine:         s.config.Engine,
			Purpose:        input.Purpose,
			IsDefault:      input.IsDefault || first,
			AutoCreateLead: input.AutoCreateLead,
			CreatedBy:      input.ActorUserID,
		})
		if err == nil {
			return session, nil
		}
		if !isUniqueViolation(err) {
			return domain.Session{}, err
		}
		lastErr = err
	}
	return domain.Session{}, fmt.Errorf("generate unique whatsapp session name: %w", lastErr)
}

func (s *SessionService) Update(ctx context.Context, scope coretenant.Scope, id string, input UpdateSessionInput) (domain.Session, error) {
	if input.Purpose != nil && !input.Purpose.IsValid() {
		return domain.Session{}, whatsappmodule.ErrInvalidPurpose
	}
	session, err := s.repo.Update(ctx, scope, id, repository.UpdateSessionParams{
		DisplayName:    input.DisplayName,
		IsDefault:      input.IsDefault,
		Purpose:        input.Purpose,
		AutoCreateLead: input.AutoCreateLead,
		UpdatedBy:      input.ActorUserID,
	})
	return session, whatsappmodule.MapSessionNotFound(err)
}

// Delete unlinks the device and removes the session from WAHA before the
// local soft delete. If WAHA deletion fails the local row is kept so the
// tenant can retry (and the session keeps counting toward the quota).
func (s *SessionService) Delete(ctx context.Context, scope coretenant.Scope, id string, actorUserID string) error {
	session, err := s.Get(ctx, scope, id)
	if err != nil {
		return err
	}
	if s.provider == nil {
		return whatsappmodule.ErrNotConfigured
	}

	// Logout fails for sessions that were never paired; deletion is what
	// matters, so the error is only logged.
	if _, err := s.provider.LogoutSession(ctx, session.Name); err != nil && !errors.Is(err, platformwhatsapp.ErrSessionNotFound) {
		s.log.Info("whatsapp: logout before delete failed", "session_id", session.ID, "error", err)
	}
	if err := s.provider.DeleteSession(ctx, session.Name); err != nil {
		return whatsappmodule.MapProviderError(err)
	}
	return whatsappmodule.MapSessionNotFound(s.repo.SoftDelete(ctx, scope, session.ID, actorUserID))
}

func (s *SessionService) Start(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error) {
	return s.providerAction(ctx, scope, id, SessionProvider.StartSession)
}

func (s *SessionService) Stop(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error) {
	return s.providerAction(ctx, scope, id, SessionProvider.StopSession)
}

// Logout unlinks the phone; the WAHA session remains and can be paired again.
func (s *SessionService) Logout(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error) {
	return s.providerAction(ctx, scope, id, SessionProvider.LogoutSession)
}

func (s *SessionService) providerAction(
	ctx context.Context,
	scope coretenant.Scope,
	id string,
	action func(SessionProvider, context.Context, string) (platformwhatsapp.SessionInfo, error),
) (domain.Session, error) {
	session, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Session{}, err
	}
	if s.provider == nil {
		return domain.Session{}, whatsappmodule.ErrNotConfigured
	}
	info, err := action(s.provider, ctx, session.Name)
	if err != nil {
		return domain.Session{}, whatsappmodule.MapProviderError(err)
	}
	return s.applyProviderInfo(ctx, scope, session, info)
}

// QR returns the pairing QR as a data URL. Only valid while WAHA waits for a
// scan; the status is refreshed first so a just-paired session is reported.
func (s *SessionService) QR(ctx context.Context, scope coretenant.Scope, id string) (SessionQR, error) {
	session, err := s.refresh(ctx, scope, id)
	if err != nil {
		return SessionQR{}, err
	}
	if session.Status != domain.SessionStatusScanQRCode {
		return SessionQR{}, whatsappmodule.ErrSessionNotScanning
	}
	qr, err := s.provider.GetQR(ctx, session.Name)
	if err != nil {
		return SessionQR{}, whatsappmodule.MapProviderError(err)
	}
	return SessionQR{DataURL: qr.DataURL(), Status: session.Status}, nil
}

// PairingCode requests a phone pairing code (alternative to scanning the QR).
func (s *SessionService) PairingCode(ctx context.Context, scope coretenant.Scope, id string, rawPhone string) (string, error) {
	normalized, ok := phone.NormalizeID(rawPhone)
	if !ok {
		return "", whatsappmodule.ErrInvalidPairingPhone
	}
	session, err := s.refresh(ctx, scope, id)
	if err != nil {
		return "", err
	}
	if session.Status != domain.SessionStatusScanQRCode {
		return "", whatsappmodule.ErrSessionNotScanning
	}
	code, err := s.provider.RequestPairingCode(ctx, session.Name, normalized)
	if err != nil {
		return "", whatsappmodule.MapProviderError(err)
	}
	return code, nil
}

// Status returns the stored session, re-reading WAHA when the stored status
// is older than StatusStaleAfter. Provider failures fall back to the stored
// status so polling UIs keep working.
func (s *SessionService) Status(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error) {
	session, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Session{}, err
	}
	if s.provider == nil || !s.isStale(session) {
		return session, nil
	}
	synced, err := s.sync(ctx, scope, session)
	if err != nil {
		s.log.Warn("whatsapp: status sync failed, returning stored status", "session_id", session.ID, "error", err)
		return session, nil
	}
	return synced, nil
}

// Reconcile re-reads one session from WAHA; used by the worker.
func (s *SessionService) Reconcile(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error) {
	if s.provider == nil {
		return domain.Session{}, whatsappmodule.ErrNotConfigured
	}
	session, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Session{}, err
	}
	return s.sync(ctx, scope, session)
}

func (s *SessionService) refresh(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error) {
	if s.provider == nil {
		return domain.Session{}, whatsappmodule.ErrNotConfigured
	}
	session, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Session{}, err
	}
	synced, err := s.sync(ctx, scope, session)
	if err != nil {
		return domain.Session{}, whatsappmodule.MapProviderError(err)
	}
	return synced, nil
}

func (s *SessionService) sync(ctx context.Context, scope coretenant.Scope, session domain.Session) (domain.Session, error) {
	info, err := s.provider.GetSession(ctx, session.Name)
	if err != nil {
		return domain.Session{}, err
	}
	return s.applyProviderInfo(ctx, scope, session, info)
}

// applyProviderInfo stores the status (and the linked number once WAHA
// reports it) returned by WAHA.
func (s *SessionService) applyProviderInfo(ctx context.Context, scope coretenant.Scope, session domain.Session, info platformwhatsapp.SessionInfo) (domain.Session, error) {
	status := domain.SessionStatus(info.Status)
	if !status.IsValid() {
		s.log.Warn("whatsapp: unknown session status from provider", "session_id", session.ID, "status", info.Status)
		return session, nil
	}
	params := repository.UpdateSessionStatusParams{Status: status, At: s.now()}
	if info.Me != nil {
		if number, ok := phone.NormalizeID(strings.SplitN(info.Me.ID, "@", 2)[0]); ok {
			params.Phone = &number
		}
		pushName := info.Me.PushName
		params.PushName = &pushName
	}
	if status != session.Status {
		s.log.Info("whatsapp: session status changed", "session_id", session.ID, "from", session.Status, "to", status)
	}
	updated, err := s.repo.UpdateStatus(ctx, scope, session.ID, params)
	return updated, whatsappmodule.MapSessionNotFound(err)
}

func (s *SessionService) isStale(session domain.Session) bool {
	return session.LastStatusAt == nil || s.now().Sub(*session.LastStatusAt) > s.config.StatusStaleAfter
}

func (s *SessionService) canCreateRemote() bool {
	return s.provider != nil &&
		strings.TrimSpace(s.config.WebhookURL) != "" &&
		strings.TrimSpace(s.config.WebhookHMACKey) != ""
}

// generateSessionName builds zc_<first 12 hex of org id>_<6 random [a-z0-9]>.
// The org part only helps operators on the shared WAHA server; ownership is
// always resolved through wa_session_directory.
func generateSessionName(organizationID string) (string, error) {
	orgPart := strings.ReplaceAll(strings.ToLower(organizationID), "-", "")
	if len(orgPart) > 12 {
		orgPart = orgPart[:12]
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	random := make([]byte, sessionNameRandomLength)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate whatsapp session name: %w", err)
	}
	for i := range random {
		random[i] = alphabet[int(random[i])%len(alphabet)]
	}
	return sessionNamePrefix + orgPart + "_" + string(random), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
