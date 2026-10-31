package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// fakeSecretRepo mirrors the documented semantics of the SQL implementation so
// the service can be tested without a database. The SQL itself is covered by
// the integration tests.
type fakeSecretRepo struct {
	rows map[string]*domain.PPPSecret // key: routerID + "/" + name
	seq  int

	pairCalls int

	listFilter            repository.SecretFilter
	listLimit, listOffset int
}

func newFakeSecretRepo() *fakeSecretRepo {
	return &fakeSecretRepo{rows: map[string]*domain.PPPSecret{}}
}

func (f *fakeSecretRepo) UpsertImported(_ context.Context, scope coretenant.Scope, routerID string, in []repository.ImportedSecret, seenAt time.Time) (repository.ImportResult, error) {
	var res repository.ImportResult
	for _, s := range in {
		key := routerID + "/" + s.Name
		row, ok := f.rows[key]
		if !ok {
			f.seq++
			row = &domain.PPPSecret{ID: fmt.Sprintf("s-%d", f.seq), OrganizationID: scope.OrganizationID(), RouterID: routerID, Name: s.Name, IsolationState: domain.IsolationNormal}
			f.rows[key] = row
			res.Created++
		} else {
			res.Updated++
		}
		row.Profile, row.Disabled, row.Comment, row.Present, row.LastSeenAt = s.Profile, s.Disabled, s.Comment, true, &seenAt
		if s.NeedsOriginalProfile && row.OriginalProfile == "" {
			row.NeedsOriginalProfile = true
		}
	}
	return res, nil
}

func (f *fakeSecretRepo) MarkMissing(_ context.Context, _ coretenant.Scope, routerID string, seenAt time.Time) (int, error) {
	n := 0
	for _, r := range f.rows {
		if r.RouterID == routerID && r.Present && (r.LastSeenAt == nil || r.LastSeenAt.Before(seenAt)) {
			r.Present = false
			n++
		}
	}
	return n, nil
}

func (f *fakeSecretRepo) FindByID(_ context.Context, scope coretenant.Scope, id string) (domain.PPPSecret, error) {
	for _, r := range f.rows {
		if r.ID == id && r.OrganizationID == scope.OrganizationID() {
			return *r, nil
		}
	}
	return domain.PPPSecret{}, mikrotik.ErrSecretNotFound
}

func (f *fakeSecretRepo) List(_ context.Context, _ coretenant.Scope, filter repository.SecretFilter, limit, offset int) ([]domain.PPPSecret, int64, error) {
	f.listFilter, f.listLimit, f.listOffset = filter, limit, offset
	return nil, 0, nil
}

func (f *fakeSecretRepo) byID(id string) *domain.PPPSecret {
	for _, r := range f.rows {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// SetOriginalProfile mirrors the SQL: it also clears needs_original_profile.
func (f *fakeSecretRepo) SetOriginalProfile(_ context.Context, _ coretenant.Scope, id, profile string) error {
	r := f.byID(id)
	if r == nil {
		return mikrotik.ErrSecretNotFound
	}
	r.OriginalProfile, r.NeedsOriginalProfile = profile, false
	return nil
}

// Pair mirrors the SQL guard (unpaired, present, not needs_original_profile).
func (f *fakeSecretRepo) Pair(_ context.Context, _ coretenant.Scope, id, accountID, by string) error {
	f.pairCalls++
	r := f.byID(id)
	switch {
	case r == nil:
		return mikrotik.ErrSecretNotFound
	case r.AccountID != nil:
		return mikrotik.ErrSecretAlreadyPaired
	case !r.Present || r.NeedsOriginalProfile:
		return mikrotik.ErrSecretNotPairable
	}
	r.AccountID, r.PairedBy = &accountID, &by
	if r.OriginalProfile == "" {
		r.OriginalProfile = r.Profile
	}
	return nil
}

func (f *fakeSecretRepo) Unpair(_ context.Context, _ coretenant.Scope, id string) error {
	r := f.byID(id)
	switch {
	case r == nil:
		return mikrotik.ErrSecretNotFound
	case r.IsolationState != domain.IsolationNormal:
		return mikrotik.ErrSecretIsolated
	}
	r.AccountID, r.PairedBy, r.PairedAt = nil, nil, nil
	return nil
}

func (f *fakeSecretRepo) byName(routerID, name string) *domain.PPPSecret {
	return f.rows[routerID+"/"+name]
}

func (f *fakeSecretRepo) names(routerID string) []string {
	var out []string
	for _, r := range f.rows {
		if r.RouterID == routerID {
			out = append(out, r.Name)
		}
	}
	sort.Strings(out)
	return out
}

type secretEnv struct {
	svc     *SecretService
	routers *RouterService
	rrepo   *fakeRepo
	srepo   *fakeSecretRepo
	client  *pmk.FakeClient
	scope   coretenant.Scope
	router  domain.Router
}

func newSecretEnv(t *testing.T) secretEnv {
	t.Helper()
	routers, rrepo, factory := newTestService(t)
	scope := testScope(t)
	router, err := routers.Register(context.Background(), scope, validInput())
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	srepo := newFakeSecretRepo()
	return secretEnv{
		svc: NewSecretService(routers, srepo, rrepo, nil, nil), routers: routers, rrepo: rrepo, srepo: srepo,
		client: factory.client, scope: scope, router: router,
	}
}

func (e secretEnv) setIsolirProfile(profile string) {
	r := e.rrepo.routers[e.router.ID]
	r.IsolirProfileName = profile
	e.rrepo.routers[e.router.ID] = r
}

func (e secretEnv) seed(rows ...pmk.Row) { e.client.Seed("/ppp/secret", rows...) }

func (e secretEnv) doImport(t *testing.T) repository.ImportResult {
	t.Helper()
	res, err := e.svc.Import(context.Background(), e.scope, e.router.ID, testActor)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	return res
}

func TestImport_CreatesSecrets(t *testing.T) {
	e := newSecretEnv(t)
	e.seed(
		pmk.Row{"name": "alice", "service": "pppoe", "profile": "paket-10m", "comment": "Alice"},
		pmk.Row{"name": "bob", "service": "pppoe", "profile": "paket-20m", "disabled": "true"},
		pmk.Row{"name": "carol", "profile": "default"},
	)
	res := e.doImport(t)
	if res.Created != 3 || res.Updated != 0 || res.Missing != 0 {
		t.Fatalf("result = %+v", res)
	}
	if bob := e.srepo.byName(e.router.ID, "bob"); bob == nil || !bob.Disabled || bob.Profile != "paket-20m" {
		t.Fatalf("bob = %+v", bob)
	}
	if len(e.rrepo.audits) < 2 {
		t.Fatalf("audits = %+v", e.rrepo.audits)
	}
	last := e.rrepo.audits[len(e.rrepo.audits)-1]
	if last.Event != "secret.imported" || last.TargetID != e.router.ID || last.Metadata["target"] != "secret" || last.ActorUserID != testActor {
		t.Fatalf("audit = %+v", last)
	}
	for k, v := range last.Metadata {
		if strings.Contains(fmt.Sprint(v), testPassword) {
			t.Fatalf("audit metadata %q leaks credentials", k)
		}
	}
}

func TestImport_ReimportKeepsPairing(t *testing.T) {
	e := newSecretEnv(t)
	e.seed(pmk.Row{"name": "alice", "service": "pppoe", "profile": "paket-10m"})
	e.doImport(t)

	acct := "acct-1"
	row := e.srepo.byName(e.router.ID, "alice")
	row.AccountID, row.OriginalProfile, row.IsolationState = &acct, "paket-10m", domain.IsolationIsolated

	// The router now reports a different profile for the same secret.
	rows := e.client.Rows("/ppp/secret")
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	id := rows[0][".id"]
	if err := e.client.Set(context.Background(), "/ppp/secret", id, pmk.Row{"profile": "isolir"}); err != nil {
		t.Fatal(err)
	}

	res := e.doImport(t)
	if res.Updated != 1 || res.Created != 0 {
		t.Fatalf("result = %+v", res)
	}
	got := e.srepo.byName(e.router.ID, "alice")
	if got.Profile != "isolir" {
		t.Fatalf("profile = %q, want updated", got.Profile)
	}
	if got.AccountID == nil || *got.AccountID != acct || got.OriginalProfile != "paket-10m" || got.IsolationState != domain.IsolationIsolated {
		t.Fatalf("pairing lost: %+v", got)
	}
}

func TestImport_MissingSecretMarkedNotDeleted(t *testing.T) {
	e := newSecretEnv(t)
	e.seed(
		pmk.Row{"name": "alice", "service": "pppoe", "profile": "p"},
		pmk.Row{"name": "bob", "service": "pppoe", "profile": "p"},
	)
	e.doImport(t)
	rows := e.client.Rows("/ppp/secret")
	for _, r := range rows {
		if r["name"] == "bob" {
			if err := e.client.Remove(context.Background(), "/ppp/secret", r[".id"]); err != nil {
				t.Fatal(err)
			}
		}
	}
	res := e.doImport(t)
	if res.Missing != 1 || res.Updated != 1 {
		t.Fatalf("result = %+v", res)
	}
	bob := e.srepo.byName(e.router.ID, "bob")
	if bob == nil || bob.Present {
		t.Fatalf("bob must remain with present=false: %+v", bob)
	}
	if !e.srepo.byName(e.router.ID, "alice").Present {
		t.Fatal("alice must stay present")
	}
}

func TestImport_IsolirProfileFlagsNeedsOriginal(t *testing.T) {
	e := newSecretEnv(t)
	e.setIsolirProfile("isolir")
	e.seed(
		pmk.Row{"name": "alice", "service": "pppoe", "profile": "isolir"},
		pmk.Row{"name": "bob", "service": "pppoe", "profile": "paket"},
	)
	e.doImport(t)
	if !e.srepo.byName(e.router.ID, "alice").NeedsOriginalProfile {
		t.Fatal("alice on the isolir profile must be flagged")
	}
	if e.srepo.byName(e.router.ID, "bob").NeedsOriginalProfile {
		t.Fatal("bob must not be flagged")
	}
}

func TestImport_NoIsolirProfileFlagsNothing(t *testing.T) {
	e := newSecretEnv(t)
	e.seed(pmk.Row{"name": "alice", "service": "pppoe", "profile": ""})
	e.doImport(t)
	if e.srepo.byName(e.router.ID, "alice").NeedsOriginalProfile {
		t.Fatal("empty isolir profile name must flag nothing")
	}
}

func TestImport_TooManySecretsRejected(t *testing.T) {
	e := newSecretEnv(t)
	rows := make([]pmk.Row, MaxImportSecrets+1)
	for i := range rows {
		rows[i] = pmk.Row{"name": fmt.Sprintf("u%d", i), "service": "pppoe", "profile": "p"}
	}
	e.seed(rows...)
	_, err := e.svc.Import(context.Background(), e.scope, e.router.ID, testActor)
	if !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	var inv *mikrotik.InvalidInputError
	if !errors.As(err, &inv) {
		t.Fatalf("err type = %T", err)
	}
	if len(e.srepo.rows) != 0 {
		t.Fatal("nothing may be stored")
	}
}

func TestImport_SkipsNonPPPoEService(t *testing.T) {
	e := newSecretEnv(t)
	e.seed(
		pmk.Row{"name": "pppoe-user", "service": "pppoe"},
		pmk.Row{"name": "any-user", "service": "any"},
		pmk.Row{"name": "no-service"},
		pmk.Row{"name": "vpn-user", "service": "l2tp"},
		pmk.Row{"name": "ovpn-user", "service": "ovpn"},
		pmk.Row{"name": "", "service": "pppoe"},
	)
	res := e.doImport(t)
	if res.Created != 3 {
		t.Fatalf("result = %+v", res)
	}
	got := strings.Join(e.srepo.names(e.router.ID), ",")
	if got != "any-user,no-service,pppoe-user" {
		t.Fatalf("stored = %s", got)
	}
}

func TestImport_UnreachableRouterScrubbed(t *testing.T) {
	e := newSecretEnv(t)
	e.client.Err = errors.New("dial failed using password " + testPassword)
	_, err := e.svc.Import(context.Background(), e.scope, e.router.ID, testActor)
	if !errors.Is(err, mikrotik.ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable", err)
	}
	if strings.Contains(err.Error(), testPassword) {
		t.Fatal("error leaks the password")
	}
	for _, a := range e.rrepo.audits {
		if strings.Contains(fmt.Sprint(a.Metadata), testPassword) {
			t.Fatal("audit leaks the password")
		}
	}
}

func TestImport_DeletedRouterNotFound(t *testing.T) {
	e := newSecretEnv(t)
	if err := e.routers.Delete(context.Background(), e.scope, e.router.ID, testActor); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Import(context.Background(), e.scope, e.router.ID, testActor); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("err = %v, want ErrRouterNotFound", err)
	}
}
