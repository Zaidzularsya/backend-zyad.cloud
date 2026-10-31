package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"testing"

	corecrypto "zyad.cloud/internal/core/crypto"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

const (
	testSecret   = "unit-test-app-secret-32-bytes-long!!"
	testPassword = "s3cret-pass-XYZ"
	testActor    = "00000000-0000-0000-0000-0000000000a1"
)

// fakeRepo is an in-memory RouterRepository.
type fakeRepo struct {
	routers map[string]domain.Router
	seq     int
	audits  []repository.AuditEntry
	checks  []string
	setups  []setupCall
	markErr error
	finds   int

	// Error injection.
	auditErr, checkErr, createErr, updateErr error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{routers: map[string]domain.Router{}} }

func (f *fakeRepo) Create(_ context.Context, scope coretenant.Scope, p repository.CreateRouterParams) (domain.Router, error) {
	if f.createErr != nil {
		return domain.Router{}, f.createErr
	}
	f.seq++
	r := domain.Router{
		ID: fmt.Sprintf("00000000-0000-0000-0000-%012d", f.seq), OrganizationID: scope.OrganizationID(),
		Name: p.Name, Host: p.Host, Port: p.Port, APIType: p.APIType, UseTLS: p.UseTLS,
		TLSFingerprint: p.TLSFingerprint, Username: p.Username, PasswordEncrypted: p.PasswordEncrypted,
		RouterOSVersion: p.RouterOSVersion,
	}
	f.routers[r.ID] = r
	return r, nil
}

func (f *fakeRepo) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Router, error) {
	f.finds++
	r, ok := f.routers[id]
	if !ok {
		return domain.Router{}, mikrotik.ErrRouterNotFound
	}
	return r, nil
}

func (f *fakeRepo) List(context.Context, coretenant.Scope, int, int) ([]domain.Router, int64, error) {
	var out []domain.Router
	for _, r := range f.routers {
		out = append(out, r)
	}
	return out, int64(len(out)), nil
}

func (f *fakeRepo) Update(_ context.Context, _ coretenant.Scope, id string, p repository.UpdateRouterParams) (domain.Router, error) {
	if f.updateErr != nil {
		return domain.Router{}, f.updateErr
	}
	r, ok := f.routers[id]
	if !ok {
		return domain.Router{}, mikrotik.ErrRouterNotFound
	}
	if p.Name != nil {
		r.Name = *p.Name
	}
	if p.Host != nil {
		r.Host = *p.Host
	}
	if p.Port != nil {
		r.Port = *p.Port
	}
	if p.APIType != nil {
		r.APIType = *p.APIType
	}
	if p.UseTLS != nil {
		r.UseTLS = *p.UseTLS
	}
	if p.TLSFingerprint != nil {
		r.TLSFingerprint = *p.TLSFingerprint
	}
	if p.Username != nil {
		r.Username = *p.Username
	}
	if p.PasswordEncrypted != nil {
		r.PasswordEncrypted = *p.PasswordEncrypted
	}
	if p.RouterOSVersion != nil {
		r.RouterOSVersion = *p.RouterOSVersion
	}
	f.routers[id] = r
	return r, nil
}

func (f *fakeRepo) RecordCheck(_ context.Context, _ coretenant.Scope, id, status, version string) error {
	if f.checkErr != nil {
		return f.checkErr
	}
	r, ok := f.routers[id]
	if !ok {
		return mikrotik.ErrRouterNotFound
	}
	r.LastCheckStatus = status
	if version != "" {
		r.RouterOSVersion = version
	}
	f.routers[id] = r
	f.checks = append(f.checks, status)
	return nil
}

type setupCall struct {
	id          string
	ready       bool
	profile     string
	redirectURL string
}

func (f *fakeRepo) MarkSetup(_ context.Context, _ coretenant.Scope, id string, ready bool, profile, redirectURL string) error {
	if f.markErr != nil {
		return f.markErr
	}
	r, ok := f.routers[id]
	if !ok {
		return mikrotik.ErrRouterNotFound
	}
	r.IsolirReady, r.IsolirProfileName, r.IsolirRedirectURL = ready, profile, redirectURL
	f.routers[id] = r
	f.setups = append(f.setups, setupCall{id, ready, profile, redirectURL})
	return nil
}

func (f *fakeRepo) SoftDelete(_ context.Context, _ coretenant.Scope, id, _ string) error {
	if _, ok := f.routers[id]; !ok {
		return mikrotik.ErrRouterNotFound
	}
	delete(f.routers, id)
	return nil
}

func (f *fakeRepo) HasReadyRouter(_ context.Context, scope coretenant.Scope) (bool, error) {
	for _, r := range f.routers {
		if r.OrganizationID == scope.OrganizationID() && r.IsolirReady {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) InsertAudit(_ context.Context, _ coretenant.Scope, e repository.AuditEntry) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.audits = append(f.audits, e)
	return nil
}

// fakeFactory returns scripted Detect results and a shared FakeClient.
type fakeFactory struct {
	detectAPI     string
	detectVersion string
	detectErr     error
	client        *pmk.FakeClient
	detectCalls   int
	lastEndpoint  pmk.Endpoint
}

func (f *fakeFactory) Detect(_ context.Context, ep pmk.Endpoint) (string, string, error) {
	f.detectCalls++
	f.lastEndpoint = ep
	if f.detectErr != nil {
		return "", "", f.detectErr
	}
	return f.detectAPI, f.detectVersion, nil
}

func (f *fakeFactory) New(_ string, ep pmk.Endpoint) (pmk.RouterClient, error) {
	f.lastEndpoint = ep
	return f.client, nil
}

func testScope(t *testing.T) coretenant.Scope {
	t.Helper()
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "00000000-0000-0000-0000-000000000201",
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceWorker,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext: %v", err)
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	return scope
}

func newTestService(t *testing.T) (*RouterService, *fakeRepo, *fakeFactory) {
	t.Helper()
	repo := newFakeRepo()
	factory := &fakeFactory{detectAPI: pmk.APITypeREST, detectVersion: "7.15", client: pmk.NewFakeClient()}
	svc := NewRouterService(repo, factory, testSecret, []netip.Prefix{netip.MustParsePrefix("10.8.0.0/24")})
	return svc, repo, factory
}

func validInput() RegisterInput {
	return RegisterInput{Name: "Core", Host: "10.8.0.2", Port: 8728, Username: "api", Password: testPassword, ActorUserID: testActor}
}

func TestRegisterRejectsPublicHostWithoutTLS(t *testing.T) {
	svc, repo, factory := newTestService(t)
	in := validInput()
	in.Host = "8.8.8.8"
	_, err := svc.Register(context.Background(), testScope(t), in)
	if !errors.Is(err, mikrotik.ErrTLSRequired) {
		t.Fatalf("err = %v, want ErrTLSRequired", err)
	}
	if factory.detectCalls != 0 || len(repo.routers) != 0 {
		t.Fatal("must not probe or persist")
	}
	in.UseTLS = true
	if _, err := svc.Register(context.Background(), testScope(t), in); err != nil {
		t.Fatalf("public host with TLS: %v", err)
	}
}

func TestRegisterRejectsDisallowedHost(t *testing.T) {
	svc, repo, factory := newTestService(t)
	for _, host := range []string{"192.168.1.1", "127.0.0.1", "169.254.169.254"} {
		in := validInput()
		in.Host = host
		_, err := svc.Register(context.Background(), testScope(t), in)
		if !errors.Is(err, mikrotik.ErrHostNotAllowed) {
			t.Fatalf("%s: err = %v, want ErrHostNotAllowed", host, err)
		}
	}
	if factory.detectCalls != 0 || len(repo.routers) != 0 {
		t.Fatal("must not probe or persist")
	}
}

func TestRegisterRejectsMalformedHostBeforeNetwork(t *testing.T) {
	svc, _, factory := newTestService(t)
	svc.validateHost = func(context.Context, string, []netip.Prefix) ([]netip.Addr, bool, error) {
		t.Fatal("validateHost must not run for malformed host")
		return nil, false, nil
	}
	for _, host := range []string{"user@10.8.0.2", "10.8.0.2/x", "10.8.0.2?a=b", "10.8.0.2#f", "10.8.0. 2", "https://10.8.0.2", "10.8.0.2:8728", "10.8.0.\t2", ""} {
		in := validInput()
		in.Host = host
		_, err := svc.Register(context.Background(), testScope(t), in)
		if !errors.Is(err, mikrotik.ErrInvalidInput) {
			t.Fatalf("host %q: err = %v, want ErrInvalidInput", host, err)
		}
	}
	if factory.detectCalls != 0 {
		t.Fatal("must not probe")
	}
}

func TestRegisterDNSFailureIsUnreachable(t *testing.T) {
	svc, repo, _ := newTestService(t)
	svc.validateHost = func(context.Context, string, []netip.Prefix) ([]netip.Addr, bool, error) {
		return nil, false, errors.New("lookup nope.example: no such host")
	}
	in := validInput()
	in.Host = "nope.example"
	_, err := svc.Register(context.Background(), testScope(t), in)
	if !errors.Is(err, mikrotik.ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable", err)
	}
	if len(repo.routers) != 0 {
		t.Fatal("must not persist")
	}
}

func TestRegisterEncryptsPasswordAndPersistsDetectedAPIType(t *testing.T) {
	svc, repo, factory := newTestService(t)
	factory.detectAPI, factory.detectVersion = pmk.APITypeBinary, "6.49"
	router, err := svc.Register(context.Background(), testScope(t), validInput())
	if err != nil {
		t.Fatal(err)
	}
	stored := repo.routers[router.ID]
	if stored.PasswordEncrypted == "" || stored.PasswordEncrypted == testPassword || strings.Contains(stored.PasswordEncrypted, testPassword) {
		t.Fatalf("password not encrypted: %q", stored.PasswordEncrypted)
	}
	plain, err := corecrypto.DecryptSecret(testSecret, stored.PasswordEncrypted)
	if err != nil || plain != testPassword {
		t.Fatalf("decrypt = %q, %v", plain, err)
	}
	if stored.APIType != domain.APITypeBinary || stored.RouterOSVersion != "6.49" {
		t.Fatalf("api/version = %s/%s", stored.APIType, stored.RouterOSVersion)
	}
	if factory.lastEndpoint.Password != testPassword || len(factory.lastEndpoint.AllowedPrivate) != 1 {
		t.Fatalf("endpoint = %+v", factory.lastEndpoint)
	}
}

func TestRegisterPropagatesTLSUntrustedWithFingerprint(t *testing.T) {
	svc, repo, factory := newTestService(t)
	factory.detectErr = fmt.Errorf("wrapped: %w", &pmk.TLSUntrustedError{Fingerprint: "abcd1234", PinMismatch: true})
	in := validInput()
	in.UseTLS = true
	_, err := svc.Register(context.Background(), testScope(t), in)
	var tlsErr *mikrotik.TLSUntrustedError
	if !errors.Is(err, mikrotik.ErrTLSUntrusted) || !errors.As(err, &tlsErr) {
		t.Fatalf("err = %v", err)
	}
	if tlsErr.Fingerprint != "abcd1234" || !tlsErr.PinMismatch {
		t.Fatalf("tlsErr = %+v", tlsErr)
	}
	if len(repo.routers) != 0 {
		t.Fatal("must not persist")
	}
}

func TestRegisterAuthFailedDoesNotPersist(t *testing.T) {
	svc, repo, factory := newTestService(t)
	factory.detectErr = pmk.ErrAuthFailed
	_, err := svc.Register(context.Background(), testScope(t), validInput())
	if !errors.Is(err, mikrotik.ErrAuthFailed) {
		t.Fatalf("err = %v", err)
	}
	if len(repo.routers) != 0 || len(repo.audits) != 0 {
		t.Fatal("must not persist or audit")
	}
}

func registerOne(t *testing.T, svc *RouterService) domain.Router {
	t.Helper()
	r, err := svc.Register(context.Background(), testScope(t), validInput())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCheckRecordsStatusOk(t *testing.T) {
	svc, repo, factory := newTestService(t)
	r := registerOne(t, svc)
	factory.client.VersionString = "7.16"
	got, err := svc.Check(context.Background(), testScope(t), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastCheckStatus != "ok" || got.RouterOSVersion != "7.16" || repo.checks[len(repo.checks)-1] != "ok" {
		t.Fatalf("router = %+v", got)
	}
	if factory.lastEndpoint.Password != testPassword {
		t.Fatal("check must decrypt the stored password for the endpoint")
	}
}

func TestCheckRecordsStatusUnreachable(t *testing.T) {
	svc, repo, factory := newTestService(t)
	r := registerOne(t, svc)
	factory.client.Err = pmk.ErrUnreachable
	got, err := svc.Check(context.Background(), testScope(t), r.ID)
	if !errors.Is(err, mikrotik.ErrUnreachable) {
		t.Fatalf("err = %v", err)
	}
	if got.LastCheckStatus != "unreachable" || repo.routers[r.ID].LastCheckStatus != "unreachable" {
		t.Fatalf("status = %q", got.LastCheckStatus)
	}
}

func TestNonUUIDIDIsNotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx, scope := context.Background(), testScope(t)
	if _, err := svc.Get(ctx, scope, "not-a-uuid"); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("Get: %v", err)
	}
	if _, err := svc.Check(ctx, scope, "x"); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("Check: %v", err)
	}
	if _, err := svc.Update(ctx, scope, "x", UpdateInput{}); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("Update: %v", err)
	}
	if err := svc.Delete(ctx, scope, "x", testActor); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("Delete: %v", err)
	}
}

func TestUpdateEndpointChangeRechecksAndEmptyPasswordKeepsOld(t *testing.T) {
	svc, repo, factory := newTestService(t)
	r := registerOne(t, svc)
	oldCipher := repo.routers[r.ID].PasswordEncrypted
	callsBefore := factory.detectCalls

	host, empty := "10.8.0.9", ""
	got, err := svc.Update(context.Background(), testScope(t), r.ID, UpdateInput{Host: &host, Password: &empty, ActorUserID: testActor})
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != host || got.PasswordEncrypted != oldCipher {
		t.Fatalf("router = %+v", got)
	}
	if factory.detectCalls != callsBefore+1 || factory.lastEndpoint.Password != testPassword {
		t.Fatal("endpoint change must re-probe with the stored password")
	}

	bad := "192.168.9.9"
	if _, err := svc.Update(context.Background(), testScope(t), r.ID, UpdateInput{Host: &bad}); !errors.Is(err, mikrotik.ErrHostNotAllowed) {
		t.Fatalf("err = %v", err)
	}
	if repo.routers[r.ID].Host != host {
		t.Fatal("rejected update must not persist")
	}

	newPass := "another-pass-123"
	got, err = svc.Update(context.Background(), testScope(t), r.ID, UpdateInput{Password: &newPass})
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := corecrypto.DecryptSecret(testSecret, got.PasswordEncrypted)
	if plain != newPass {
		t.Fatal("new password must be stored encrypted")
	}
}

func TestUpdateNameOnlySkipsProbe(t *testing.T) {
	svc, _, factory := newTestService(t)
	r := registerOne(t, svc)
	before := factory.detectCalls
	name := "Renamed"
	if _, err := svc.Update(context.Background(), testScope(t), r.ID, UpdateInput{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if factory.detectCalls != before {
		t.Fatal("name-only update must not probe")
	}
}

func TestAuditEventsRecorded(t *testing.T) {
	svc, repo, _ := newTestService(t)
	ctx, scope := context.Background(), testScope(t)
	r := registerOne(t, svc)
	name := "Renamed"
	if _, err := svc.Update(ctx, scope, r.ID, UpdateInput{Name: &name, ActorUserID: testActor}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, scope, r.ID, testActor); err != nil {
		t.Fatal(err)
	}
	var events []string
	for _, a := range repo.audits {
		events = append(events, a.Event)
		if a.ActorUserID != testActor || a.TargetID != r.ID {
			t.Fatalf("audit = %+v", a)
		}
		for k := range a.Metadata {
			if k != "name" && k != "host" && k != "port" {
				t.Fatalf("unexpected metadata key %q", k)
			}
		}
		if strings.Contains(fmt.Sprint(a.Metadata), testPassword) {
			t.Fatal("audit leaks password")
		}
	}
	if strings.Join(events, ",") != "router.created,router.updated,router.deleted" {
		t.Fatalf("events = %v", events)
	}
}

func TestBookkeepingFailuresDoNotFailCommittedRequests(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	svc, repo, _ := newTestService(t)
	ctx, scope := context.Background(), testScope(t)
	repo.auditErr = errors.New("audit table down")

	r, err := svc.Register(ctx, scope, validInput())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := repo.routers[r.ID]; !ok {
		t.Fatal("router must be persisted")
	}

	name := "Renamed"
	if _, err := svc.Update(ctx, scope, r.ID, UpdateInput{Name: &name}); err != nil || repo.routers[r.ID].Name != name {
		t.Fatalf("Update: %v", err)
	}

	repo.checkErr = errors.New("check table down")
	host := "10.8.0.7"
	if _, err := svc.Update(ctx, scope, r.ID, UpdateInput{Host: &host}); err != nil || repo.routers[r.ID].Host != host {
		t.Fatalf("Update with failing RecordCheck: %v", err)
	}

	if err := svc.Delete(ctx, scope, r.ID, testActor); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := repo.routers[r.ID]; ok {
		t.Fatal("router must be deleted")
	}
	out := logs.String()
	for _, want := range []string{"audit insert failed", "router.created", "router.deleted", "record check after update failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, testPassword) {
		t.Fatal("log leaks password")
	}
}

func assertNoSecret(t *testing.T, label string, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected an error", label)
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(err.Error(), secret) {
			t.Fatalf("%s: error leaks a secret: %v", label, err)
		}
	}
}

func TestErrorsNeverContainPassword(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	leaky := []error{
		errors.New("boom: " + testPassword),
		fmt.Errorf("wrapped: %w", errors.New("inner "+testPassword)),
		fmt.Errorf("auth with %s: %w", testPassword, pmk.ErrAuthFailed),
	}
	ctx, scope := context.Background(), testScope(t)

	for i, leak := range leaky {
		// Register: Detect error.
		svc, repo, factory := newTestService(t)
		factory.detectErr = leak
		_, err := svc.Register(ctx, scope, validInput())
		assertNoSecret(t, fmt.Sprintf("Register/Detect[%d]", i), err, testPassword)

		// Register: repository error echoing both secrets.
		svc, repo, _ = newTestService(t)
		repo.createErr = errors.New("insert failed for " + testPassword)
		_, err = svc.Register(ctx, scope, validInput())
		assertNoSecret(t, fmt.Sprintf("Register/Create[%d]", i), err, testPassword)

		// Check: client Version error.
		svc, repo, factory = newTestService(t)
		r := registerOne(t, svc)
		enc := repo.routers[r.ID].PasswordEncrypted
		factory.client.Err = leak
		_, err = svc.Check(ctx, scope, r.ID)
		assertNoSecret(t, fmt.Sprintf("Check[%d]", i), err, testPassword, enc)

		// Update: probe error and repository error.
		factory.detectErr = leak
		host := "10.8.0.8"
		_, err = svc.Update(ctx, scope, r.ID, UpdateInput{Host: &host})
		assertNoSecret(t, fmt.Sprintf("Update/Detect[%d]", i), err, testPassword, enc)

		factory.detectErr = nil
		repo.updateErr = errors.New("update failed " + testPassword + " " + enc)
		newPass := "new-" + testPassword
		_, err = svc.Update(ctx, scope, r.ID, UpdateInput{Password: &newPass})
		assertNoSecret(t, fmt.Sprintf("Update/Store[%d]", i), err, testPassword, enc, newPass)
	}

	// Decrypt failure (corrupt ciphertext) on Check and Update.
	svc, repo, _ := newTestService(t)
	r := registerOne(t, svc)
	corrupt := repo.routers[r.ID]
	corrupt.PasswordEncrypted = "corrupt-" + testPassword
	repo.routers[r.ID] = corrupt
	_, err := svc.Check(ctx, scope, r.ID)
	assertNoSecret(t, "Check/decrypt", err, testPassword, corrupt.PasswordEncrypted)
	host := "10.8.0.8"
	_, err = svc.Update(ctx, scope, r.ID, UpdateInput{Host: &host})
	assertNoSecret(t, "Update/decrypt", err, testPassword, corrupt.PasswordEncrypted)

	// Unclassified errors are the unreachable class with a fixed message.
	svc, _, factory := newTestService(t)
	factory.detectErr = leaky[0]
	_, err = svc.Register(ctx, scope, validInput())
	if !errors.Is(err, mikrotik.ErrUnreachable) || err.Error() != mikrotik.ErrUnreachable.Error() {
		t.Fatalf("unclassified error = %v, want fixed ErrUnreachable", err)
	}
	if strings.Contains(logs.String(), testPassword) {
		t.Fatalf("logs leak password:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "***") {
		t.Fatal("expected scrubbed original in log")
	}
}
