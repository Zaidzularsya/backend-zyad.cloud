package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"regexp"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"

	corecrypto "zyad.cloud/internal/core/crypto"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// Check outcomes stored in last_check_status.
const (
	CheckStatusOK           = "ok"
	CheckStatusUnreachable  = "unreachable"
	CheckStatusAuthFailed   = "auth_failed"
	CheckStatusTLSUntrusted = "tls_untrusted"
	CheckStatusHostBlocked  = "host_not_allowed"
)

const maxNameLength = 100

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ClientFactory is the seam between the service and the router transport, so
// tests can inject pmk.FakeClient.
type ClientFactory interface {
	Detect(ctx context.Context, ep pmk.Endpoint) (apiType, version string, err error)
	New(apiType string, ep pmk.Endpoint) (pmk.RouterClient, error)
}

type platformFactory struct{}

func (platformFactory) Detect(ctx context.Context, ep pmk.Endpoint) (string, string, error) {
	return pmk.Detect(ctx, ep)
}

func (platformFactory) New(apiType string, ep pmk.Endpoint) (pmk.RouterClient, error) {
	return pmk.NewClient(apiType, ep)
}

// NewPlatformClientFactory returns the production ClientFactory.
func NewPlatformClientFactory() ClientFactory { return platformFactory{} }

type RegisterInput struct {
	Name           string
	Host           string
	Port           int
	Username       string
	Password       string
	UseTLS         bool
	TLSFingerprint string
	ActorUserID    string
}

// UpdateInput is a partial update: nil means unchanged. An empty Password also
// means unchanged.
type UpdateInput struct {
	Name           *string
	Host           *string
	Port           *int
	Username       *string
	Password       *string
	UseTLS         *bool
	TLSFingerprint *string
	ActorUserID    string
}

type RouterService struct {
	repo           repository.RouterRepository
	factory        ClientFactory
	secret         string
	allowedPrivate []netip.Prefix
	validateHost   func(ctx context.Context, host string, allowed []netip.Prefix) ([]netip.Addr, bool, error)
}

func NewRouterService(repo repository.RouterRepository, factory ClientFactory, secret string, allowedPrivate []netip.Prefix) *RouterService {
	return &RouterService{
		repo:           repo,
		factory:        factory,
		secret:         secret,
		allowedPrivate: allowedPrivate,
		validateHost:   pmk.ValidateHost,
	}
}

func invalid(msg string) error { return &mikrotik.InvalidInputError{Message: msg} }

func validID(id string) bool { return uuidPattern.MatchString(id) }

// validateHostSyntax rejects anything but a bare hostname or IP literal: URLs,
// credentials, paths and host:port belong elsewhere (the Port field).
func validateHostSyntax(host string) error {
	if host == "" {
		return invalid("host is required")
	}
	if strings.ContainsAny(host, "@/?#\\ \t\r\n") {
		return invalid("host must be a bare hostname or IP address")
	}
	if strings.Contains(host, ":") {
		if _, err := netip.ParseAddr(host); err != nil {
			return invalid("host must not contain a scheme or port; use the port field")
		}
	}
	if strings.ContainsAny(host, "[]") {
		return invalid("host must be a bare hostname or IP address")
	}
	return nil
}

func validatePort(port int) error {
	if port < 0 || port > 65535 {
		return invalid("port must be between 1 and 65535")
	}
	return nil
}

func defaultPort(port int, useTLS bool) int {
	if port != 0 {
		return port
	}
	if useTLS {
		return 443
	}
	return 80
}

// checkHostPolicy resolves the host and applies the network policy: the host
// must be public or inside the allow list, and a public host requires TLS.
func (s *RouterService) checkHostPolicy(ctx context.Context, host string, useTLS bool) error {
	_, public, err := s.validateHost(ctx, host, s.allowedPrivate)
	if err != nil {
		if errors.Is(err, pmk.ErrHostNotAllowed) {
			return mikrotik.ErrHostNotAllowed
		}
		// Resolution failure: the router cannot be reached from here.
		return mikrotik.ErrUnreachable
	}
	if public && !useTLS {
		return mikrotik.ErrTLSRequired
	}
	return nil
}

// scrub replaces every non-empty secret in msg with "***".
func scrub(msg string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "***")
		}
	}
	return msg
}

// translateClientError maps platform errors to module errors. Unknown errors
// are not trusted to be free of credentials: they are logged scrubbed and
// replaced by the fixed unreachable-class error.
func translateClientError(err error, secrets ...string) error {
	var tlsErr *pmk.TLSUntrustedError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &tlsErr):
		return &mikrotik.TLSUntrustedError{Fingerprint: tlsErr.Fingerprint, PinMismatch: tlsErr.PinMismatch}
	case errors.Is(err, pmk.ErrHostNotAllowed):
		return mikrotik.ErrHostNotAllowed
	case errors.Is(err, pmk.ErrAuthFailed):
		return mikrotik.ErrAuthFailed
	case errors.Is(err, pmk.ErrUnreachable):
		return mikrotik.ErrUnreachable
	default:
		slog.Warn("mikrotik: unclassified router error", "error", scrub(err.Error(), secrets...), "type", fmt.Sprintf("%T", err))
		return mikrotik.ErrUnreachable
	}
}

// storeError keeps known repository sentinels and replaces anything else with a
// fixed message, since the raw error could echo the values we tried to store.
func storeError(err error, secrets ...string) error {
	if errors.Is(err, mikrotik.ErrRouterNotFound) || errors.Is(err, mikrotik.ErrRouterNameTaken) {
		return err
	}
	slog.Warn("mikrotik: router store failed", "error", scrub(err.Error(), secrets...), "type", fmt.Sprintf("%T", err))
	return errors.New("store router failed")
}

// audit writes an audit entry after a mutation has already been committed.
// The repository has no transaction shared with this service, so a failure here
// cannot undo the mutation; it is logged instead of failing a request that
// actually succeeded.
func (s *RouterService) audit(ctx context.Context, scope coretenant.Scope, entry repository.AuditEntry) {
	if err := s.repo.InsertAudit(ctx, scope, entry); err != nil {
		slog.Warn("mikrotik: audit insert failed", "router_id", entry.TargetID, "event", entry.Event, "error", scrub(err.Error()))
	}
}

func checkStatusFor(err error) string {
	switch {
	case err == nil:
		return CheckStatusOK
	case errors.Is(err, mikrotik.ErrTLSUntrusted):
		return CheckStatusTLSUntrusted
	case errors.Is(err, mikrotik.ErrAuthFailed):
		return CheckStatusAuthFailed
	case errors.Is(err, mikrotik.ErrHostNotAllowed):
		return CheckStatusHostBlocked
	default:
		return CheckStatusUnreachable
	}
}

func (s *RouterService) endpoint(host string, port int, useTLS bool, fingerprint, username, password string) pmk.Endpoint {
	return pmk.Endpoint{
		Host:           host,
		Port:           port,
		Username:       username,
		Password:       password,
		UseTLS:         useTLS,
		TLSFingerprint: fingerprint,
		AllowedPrivate: s.allowedPrivate,
	}
}

func auditMetadata(r domain.Router) map[string]any {
	return map[string]any{"name": r.Name, "host": r.Host, "port": r.Port}
}

// Register validates and probes a router, then stores it with an encrypted
// password. Nothing is persisted when the probe fails.
func (s *RouterService) Register(ctx context.Context, scope coretenant.Scope, in RegisterInput) (domain.Router, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Host = strings.TrimSpace(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	in.TLSFingerprint = strings.TrimSpace(in.TLSFingerprint)
	switch {
	case in.Name == "" || len(in.Name) > maxNameLength:
		return domain.Router{}, invalid("name is required and must be at most 100 characters")
	case in.Username == "":
		return domain.Router{}, invalid("username is required")
	case in.Password == "":
		return domain.Router{}, invalid("password is required")
	}
	if err := validateHostSyntax(in.Host); err != nil {
		return domain.Router{}, err
	}
	if err := validatePort(in.Port); err != nil {
		return domain.Router{}, err
	}
	in.Port = defaultPort(in.Port, in.UseTLS)

	if err := s.checkHostPolicy(ctx, in.Host, in.UseTLS); err != nil {
		return domain.Router{}, err
	}
	apiType, version, err := s.factory.Detect(ctx, s.endpoint(in.Host, in.Port, in.UseTLS, in.TLSFingerprint, in.Username, in.Password))
	if err != nil {
		return domain.Router{}, translateClientError(err, in.Password)
	}
	encrypted, err := corecrypto.EncryptSecret(s.secret, in.Password)
	if err != nil {
		return domain.Router{}, errors.New("encrypt router password")
	}
	router, err := s.repo.Create(ctx, scope, repository.CreateRouterParams{
		Name:              in.Name,
		Host:              in.Host,
		Port:              in.Port,
		APIType:           domain.APIType(apiType),
		UseTLS:            in.UseTLS,
		TLSFingerprint:    in.TLSFingerprint,
		Username:          in.Username,
		PasswordEncrypted: encrypted,
		RouterOSVersion:   version,
		CreatedBy:         in.ActorUserID,
	})
	if err != nil {
		return domain.Router{}, storeError(err, in.Password, encrypted)
	}
	s.audit(ctx, scope, repository.AuditEntry{
		Event: "router.created", ActorUserID: in.ActorUserID, TargetID: router.ID, Metadata: auditMetadata(router),
	})
	return router, nil
}

func (s *RouterService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Router, error) {
	if !validID(id) {
		return domain.Router{}, mikrotik.ErrRouterNotFound
	}
	return s.repo.FindByID(ctx, scope, id)
}

func (s *RouterService) List(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]domain.Router, int64, error) {
	return s.repo.List(ctx, scope, limit, offset)
}

// Check probes the stored router and records the outcome. On a failed probe it
// returns the refreshed router together with the translated error, so callers
// can both report the stored status and surface details such as the TLS
// fingerprint.
func (s *RouterService) Check(ctx context.Context, scope coretenant.Scope, id string) (domain.Router, error) {
	router, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Router{}, err
	}
	version, checkErr := s.probe(ctx, router)
	if err := s.repo.RecordCheck(ctx, scope, id, checkStatusFor(checkErr), version); err != nil {
		return domain.Router{}, err
	}
	refreshed, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Router{}, err
	}
	return refreshed, checkErr
}

func (s *RouterService) probe(ctx context.Context, router domain.Router) (string, error) {
	client, password, err := s.dial(ctx, router)
	if err != nil {
		return "", err
	}
	version, err := client.Version(ctx)
	if err != nil {
		return "", translateClientError(err, password, router.PasswordEncrypted)
	}
	return version, nil
}

// dial decrypts the stored password, applies the host policy and builds a
// client for the stored router. The plaintext password is returned only so the
// caller (same package) can scrub it from errors and logs; it must never be
// logged, stored or returned to API clients.
func (s *RouterService) dial(ctx context.Context, router domain.Router) (pmk.RouterClient, string, error) {
	password, err := corecrypto.DecryptSecret(s.secret, router.PasswordEncrypted)
	if err != nil {
		return nil, "", errors.New("decrypt router password")
	}
	if err := s.checkHostPolicy(ctx, router.Host, router.UseTLS); err != nil {
		return nil, "", err
	}
	client, err := s.factory.New(string(router.APIType), s.endpoint(router.Host, router.Port, router.UseTLS, router.TLSFingerprint, router.Username, password))
	if err != nil {
		return nil, "", mikrotik.ErrUnreachable
	}
	return client, password, nil
}

// openClient loads a router by id and opens a RouterOS client for it. secrets
// lists every value that must be scrubbed from anything derived from errors.
func (s *RouterService) openClient(ctx context.Context, scope coretenant.Scope, id string) (domain.Router, pmk.RouterClient, []string, error) {
	router, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Router{}, nil, nil, err
	}
	client, password, err := s.dial(ctx, router)
	if err != nil {
		return domain.Router{}, nil, nil, err
	}
	return router, client, []string{password, router.PasswordEncrypted}, nil
}

// Update applies a partial update. Changing the endpoint or credentials
// re-validates the host policy and re-probes the router before saving.
func (s *RouterService) Update(ctx context.Context, scope coretenant.Scope, id string, in UpdateInput) (domain.Router, error) {
	current, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Router{}, err
	}
	params := repository.UpdateRouterParams{UpdatedBy: in.ActorUserID}
	next := current

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > maxNameLength {
			return domain.Router{}, invalid("name is required and must be at most 100 characters")
		}
		params.Name, next.Name = &name, name
	}
	if in.Username != nil {
		username := strings.TrimSpace(*in.Username)
		if username == "" {
			return domain.Router{}, invalid("username is required")
		}
		params.Username, next.Username = &username, username
	}
	if in.Host != nil {
		host := strings.TrimSpace(*in.Host)
		if err := validateHostSyntax(host); err != nil {
			return domain.Router{}, err
		}
		params.Host, next.Host = &host, host
	}
	if in.Port != nil {
		if err := validatePort(*in.Port); err != nil {
			return domain.Router{}, err
		}
	}
	if in.UseTLS != nil {
		params.UseTLS, next.UseTLS = in.UseTLS, *in.UseTLS
	}
	if in.TLSFingerprint != nil {
		fp := strings.TrimSpace(*in.TLSFingerprint)
		params.TLSFingerprint, next.TLSFingerprint = &fp, fp
	}
	if in.Port != nil {
		port := *in.Port
		if port == 0 {
			port = defaultPort(0, next.UseTLS)
		}
		params.Port, next.Port = &port, port
	}

	newPassword := ""
	if in.Password != nil {
		newPassword = *in.Password
	}
	connectionChanged := params.Host != nil || params.Port != nil || params.UseTLS != nil ||
		params.TLSFingerprint != nil || params.Username != nil || newPassword != ""

	if connectionChanged {
		password := newPassword
		if password == "" {
			if password, err = corecrypto.DecryptSecret(s.secret, current.PasswordEncrypted); err != nil {
				return domain.Router{}, errors.New("decrypt router password")
			}
		} else {
			encrypted, err := corecrypto.EncryptSecret(s.secret, password)
			if err != nil {
				return domain.Router{}, errors.New("encrypt router password")
			}
			params.PasswordEncrypted = &encrypted
		}
		if err := s.checkHostPolicy(ctx, next.Host, next.UseTLS); err != nil {
			return domain.Router{}, err
		}
		apiType, version, err := s.factory.Detect(ctx, s.endpoint(next.Host, next.Port, next.UseTLS, next.TLSFingerprint, next.Username, password))
		if err != nil {
			return domain.Router{}, translateClientError(err, password, current.PasswordEncrypted, derefStr(params.PasswordEncrypted))
		}
		at := domain.APIType(apiType)
		params.APIType, params.RouterOSVersion = &at, &version
	}

	updated, err := s.repo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.Router{}, storeError(err, newPassword, s.storedPassword(current), derefStr(params.PasswordEncrypted), current.PasswordEncrypted)
	}
	if connectionChanged {
		// Bookkeeping after a committed update (no shared transaction): log, do not fail.
		if err := s.repo.RecordCheck(ctx, scope, id, CheckStatusOK, updated.RouterOSVersion); err != nil {
			slog.Warn("mikrotik: record check after update failed", "router_id", id, "error", scrub(err.Error()))
		}
	}
	s.audit(ctx, scope, repository.AuditEntry{
		Event: "router.updated", ActorUserID: in.ActorUserID, TargetID: id, Metadata: auditMetadata(updated),
	})
	return updated, nil
}

// Delete soft-deletes the router; by is the acting user id.
func (s *RouterService) Delete(ctx context.Context, scope coretenant.Scope, id, by string) error {
	router, err := s.Get(ctx, scope, id)
	if err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, scope, id, by); err != nil {
		return err
	}
	s.audit(ctx, scope, repository.AuditEntry{
		Event: "router.deleted", ActorUserID: by, TargetID: id, Metadata: auditMetadata(router),
	})
	return nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// storedPassword decrypts the stored password only so it can be scrubbed from
// logs; failures yield an empty string.
func (s *RouterService) storedPassword(r domain.Router) string {
	plain, err := corecrypto.DecryptSecret(s.secret, r.PasswordEncrypted)
	if err != nil {
		return ""
	}
	return plain
}
