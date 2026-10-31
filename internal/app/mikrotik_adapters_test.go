package app

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/config"
	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/receivable"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/pricing"
)

func mkScope(org string) coretenant.Scope {
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: org, OrganizationSlug: "s-" + org,
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource: coretenant.ResolutionSourceWorker, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		panic(err)
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		panic(err)
	}
	return scope
}

// fakeAccounts is org-scoped like the real repository: an id only resolves in its own organization.
type fakeAccounts struct {
	byOrg      map[string]map[string]receivabledomain.Account
	ensured    []receivableservice.AccountInput
	ensureErr  error
	getErr     error
	listSearch string
	listLimit  int
}

func (f *fakeAccounts) Get(_ context.Context, s coretenant.Scope, id string) (receivabledomain.Account, error) {
	if f.getErr != nil {
		return receivabledomain.Account{}, f.getErr
	}
	if a, ok := f.byOrg[s.OrganizationID()][id]; ok {
		return a, nil
	}
	return receivabledomain.Account{}, receivable.ErrAccountNotFound
}
func (f *fakeAccounts) List(_ context.Context, s coretenant.Scope, q string, limit, _ int) ([]receivabledomain.Account, int64, error) {
	f.listSearch, f.listLimit = q, limit
	var out []receivabledomain.Account
	for _, a := range f.byOrg[s.OrganizationID()] {
		out = append(out, a)
	}
	return out, int64(len(out)), nil
}
func (f *fakeAccounts) EnsureForSource(_ context.Context, _ coretenant.Scope, in receivableservice.AccountInput, _ string) (receivabledomain.Account, error) {
	f.ensured = append(f.ensured, in)
	if f.ensureErr != nil {
		return receivabledomain.Account{}, f.ensureErr
	}
	return receivabledomain.Account{ID: "acc-new", Name: in.Name}, nil
}

func TestAccountDirectory_OrgIsolationAndNotFound(t *testing.T) {
	f := &fakeAccounts{byOrg: map[string]map[string]receivabledomain.Account{
		"org-a": {"a1": {ID: "a1", Name: "Budi", Phone: "08", Email: "b@x.id"}},
	}}
	d := mikrotikAccountDirectory{accounts: f}
	ref, err := d.Get(context.Background(), mkScope("org-a"), "a1")
	if err != nil || ref.ID != "a1" || ref.Name != "Budi" || ref.Phone != "08" || ref.Email != "b@x.id" {
		t.Fatalf("own org: %+v %v", ref, err)
	}
	if _, err := d.Get(context.Background(), mkScope("org-b"), "a1"); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("other org must be not found, got %v", err)
	}
	f.getErr = pgx.ErrNoRows
	if _, err := d.Get(context.Background(), mkScope("org-a"), "a1"); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("pgx.ErrNoRows must map, got %v", err)
	}
	boom := errors.New("db down")
	f.getErr = boom
	if _, err := d.Get(context.Background(), mkScope("org-a"), "a1"); !errors.Is(err, boom) {
		t.Fatalf("other errors pass through, got %v", err)
	}
	list, err := d.Search(context.Background(), mkScope("org-b"), "bu", 10)
	if err != nil || len(list) != 0 || f.listSearch != "bu" || f.listLimit != 10 {
		t.Fatalf("search: %v %v %q %d", list, err, f.listSearch, f.listLimit)
	}
}

type fakeContacts struct {
	got *crmrepo.CreateContactParams
	err error
}

func (f *fakeContacts) Create(_ context.Context, _ coretenant.Scope, p crmrepo.CreateContactParams) (crmdomain.Contact, error) {
	f.got = &p
	if f.err != nil {
		return crmdomain.Contact{}, f.err
	}
	return crmdomain.Contact{ID: "contact-1"}, nil
}

func TestContactProvisioner_CreatesContactThenAccount(t *testing.T) {
	acc, ct := &fakeAccounts{}, &fakeContacts{}
	p := mikrotikContactProvisioner{contacts: ct, accounts: acc}
	ref, err := p.CreateCustomerFromSecret(context.Background(), mkScope("org-a"), " budi ", "c", "user-1")
	if err != nil || ref.ID != "acc-new" || ref.Name != "budi" {
		t.Fatalf("ref=%+v err=%v", ref, err)
	}
	g := ct.got
	if g.FirstName != "budi" || g.Source != "mikrotik_import" || !g.IsCustomer || g.LifecycleStage != crmdomain.ContactLifecycleCustomer ||
		len(g.Tags) != 1 || g.Tags[0] != "pppoe" || g.CreatedBy != "user-1" {
		t.Fatalf("contact params: %+v", g)
	}
	if len(acc.ensured) != 1 || acc.ensured[0].SourceType != receivabledomain.SourceTypeCRMContact || acc.ensured[0].SourceID != "contact-1" {
		t.Fatalf("ensure: %+v", acc.ensured)
	}
}

func TestContactProvisioner_ErrorsPassThrough(t *testing.T) {
	quota := coreerrors.New("QUOTA_EXCEEDED", "billing quota has been exceeded", 409)
	acc := &fakeAccounts{}
	_, err := mikrotikContactProvisioner{contacts: &fakeContacts{err: quota}, accounts: acc}.
		CreateCustomerFromSecret(context.Background(), mkScope("o"), "x", "", "u")
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "QUOTA_EXCEEDED" {
		t.Fatalf("quota error must pass through, got %v", err)
	}
	if len(acc.ensured) != 0 {
		t.Fatal("account must not be created when contact creation fails")
	}
	boom := errors.New("ensure failed")
	acc.ensureErr = boom
	if _, err := (mikrotikContactProvisioner{contacts: &fakeContacts{}, accounts: acc}).
		CreateCustomerFromSecret(context.Background(), mkScope("o"), "x", "", "u"); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

type fakeProducts struct {
	byOrg map[string]map[string]catalogdomain.Product
}

func (f fakeProducts) Get(_ context.Context, s coretenant.Scope, id string) (catalogdomain.Product, error) {
	if p, ok := f.byOrg[s.OrganizationID()][id]; ok {
		return p, nil
	}
	return catalogdomain.Product{}, coreerrors.New("PRODUCT_NOT_FOUND", "product not found or already deleted", 404)
}
func (f fakeProducts) FindByIDs(_ context.Context, s coretenant.Scope, ids []string) (map[string]catalogdomain.Product, error) {
	out := map[string]catalogdomain.Product{}
	for _, id := range ids {
		if p, ok := f.byOrg[s.OrganizationID()][id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

type fakeInUse struct{ ids []string }

func (f fakeInUse) ActiveContractProductIDs(context.Context, coretenant.Scope) ([]string, error) {
	return f.ids, nil
}

func TestProductCatalog(t *testing.T) {
	rec := pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}
	one := pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}
	src := fakeProducts{byOrg: map[string]map[string]catalogdomain.Product{"org-a": {
		"p2": {ID: "p2", Name: "Zeta", SKU: "Z", Pricing: rec, IsActive: true},
		"p1": {ID: "p1", Name: "alpha", Pricing: rec},
		"p3": {ID: "p3", Name: "Setup", Pricing: one, IsActive: true},
	}}}
	c := mikrotikProductCatalog{products: src, inUse: fakeInUse{ids: []string{"p1", "p2", "p3", "pX"}}}

	info, err := c.Get(context.Background(), mkScope("org-a"), "p2")
	if err != nil || !info.Recurring || !info.Active || info.SKU != "Z" {
		t.Fatalf("get: %+v %v", info, err)
	}
	if _, err := c.Get(context.Background(), mkScope("org-b"), "p2"); !errors.Is(err, mikrotik.ErrProductNotFound) {
		t.Fatalf("other org must be not found, got %v", err)
	}
	list, err := c.ListRecurringInUse(context.Background(), mkScope("org-a"))
	if err != nil || len(list) != 2 || list[0].ID != "p1" || list[1].ID != "p2" {
		t.Fatalf("recurring only, sorted by name: %+v %v", list, err)
	}
	list, _ = c.ListRecurringInUse(context.Background(), mkScope("org-b"))
	if len(list) != 0 {
		t.Fatalf("other org sees nothing: %+v", list)
	}
}

func TestBuildMikrotik_WiresAllHandlers(t *testing.T) {
	// Repositories only keep the pool pointer; nothing touches the database here.
	m := buildMikrotik(config.Config{}, nil, mikrotikBuild{
		Accounts: &fakeAccounts{}, Contacts: &fakeContacts{}, Products: fakeProducts{},
	})
	if m.Router == nil || m.Setup == nil || m.Secret == nil || m.Policy == nil {
		t.Fatalf("handlers must all be built: %+v", m)
	}
}
