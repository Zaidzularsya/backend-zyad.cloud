package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/shared/pricing"
)

type fakeProducts struct {
	repository.ProductRepository
	created  []repository.CreateProductParams
	updated  []repository.UpdateProductParams
	existing *domain.Product
	byID     map[string]domain.Product
	listed   []domain.Product
}

var tenantMax = 10

type fakeRegistry struct{ defs map[string]domain.FeatureDef }

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{defs: map[string]domain.FeatureDef{
		"crm":   {Key: "crm", Name: "CRM", ValueType: "boolean", IsActive: true},
		"users": {Key: "users", Name: "Jumlah user", ValueType: "integer", Unit: "user", IsActive: true},
		"wa":    {Key: "wa", Name: "WhatsApp", ValueType: "boolean", IsActive: true},
		"old":   {Key: "old", Name: "Lama", ValueType: "boolean", IsActive: false},
		"tcrm":  {Key: "tcrm", Name: "CRM tenant", ValueType: "boolean", IsActive: true, TenantOfferable: true},
		"tusr":  {Key: "tusr", Name: "User tenant", ValueType: "integer", IsActive: true, TenantOfferable: true, TenantMax: &tenantMax},
	}}
}

func (f *fakeRegistry) ListActive(context.Context) ([]domain.FeatureDef, error) {
	var out []domain.FeatureDef
	for _, d := range f.defs {
		if d.IsActive {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeRegistry) FindByKeys(_ context.Context, keys []string) (map[string]domain.FeatureDef, error) {
	out := map[string]domain.FeatureDef{}
	for _, k := range keys {
		if d, ok := f.defs[k]; ok {
			out[k] = d
		}
	}
	return out, nil
}

func (f *fakeProducts) Create(_ context.Context, _ coretenant.Scope, p repository.CreateProductParams) (domain.Product, error) {
	f.created = append(f.created, p)
	return domain.Product{ID: "p1", Name: p.Name, Pricing: p.Pricing}, nil
}

func (f *fakeProducts) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Product, error) {
	if f.existing != nil {
		return *f.existing, nil
	}
	return domain.Product{ID: id}, nil
}

func (f *fakeProducts) FindByIDs(_ context.Context, _ coretenant.Scope, ids []string) (map[string]domain.Product, error) {
	out := map[string]domain.Product{}
	for _, id := range ids {
		if p, ok := f.byID[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func (f *fakeProducts) List(context.Context, coretenant.Scope, repository.ProductListFilter) ([]domain.Product, int64, error) {
	return f.listed, int64(len(f.listed)), nil
}

func (f *fakeProducts) Update(_ context.Context, _ coretenant.Scope, id string, p repository.UpdateProductParams) (domain.Product, error) {
	f.updated = append(f.updated, p)
	return domain.Product{ID: id}, nil
}

// Service tidak memvalidasi scope (tugas repository); fake mengabaikannya.
func scope(t *testing.T) coretenant.Scope {
	t.Helper()
	return coretenant.Scope{}
}

func TestValidators(t *testing.T) {
	for _, ok := range []string{"0", "350000", "350000.5", "350000.50"} {
		if !ValidMoney(ok) {
			t.Errorf("ValidMoney(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "-1", "1.234", "1,5", "Rp1"} {
		if ValidMoney(bad) {
			t.Errorf("ValidMoney(%q) = true", bad)
		}
	}
	for _, ok := range []string{"0", "11", "12.5", "100"} {
		if !ValidPercent(ok) {
			t.Errorf("ValidPercent(%q) = false", ok)
		}
	}
	for _, bad := range []string{"100.01", "-1", "abc", ""} {
		if ValidPercent(bad) {
			t.Errorf("ValidPercent(%q) = true", bad)
		}
	}
}

func TestCreateProductValidatesAndDefaults(t *testing.T) {
	repo := &fakeProducts{}
	svc := NewProductService(repo, newFakeRegistry())
	if _, err := svc.Create(context.Background(), scope(t), ProductInput{Name: " ", Unit: "pcs", BasePrice: "1"}, "u1"); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("blank name err = %v", err)
	}
	if _, err := svc.Create(context.Background(), scope(t), ProductInput{Name: "A", Unit: "pcs", BasePrice: "1", TaxPercent: "101"}, "u1"); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("tax err = %v", err)
	}
	if _, err := svc.Create(context.Background(), scope(t), ProductInput{Name: " Internet ", Unit: "", BasePrice: "", IsActive: true}, "u1"); err != nil {
		t.Fatal(err)
	}
	got := repo.created[0]
	if got.Name != "Internet" || got.Unit != "pcs" || got.BasePrice != "0" || got.TaxPercent != "0" || got.CreatedBy != "u1" {
		t.Fatalf("defaults = %+v", got)
	}
}

func TestUpdateProductValidatesOnlyProvidedFields(t *testing.T) {
	repo := &fakeProducts{}
	svc := NewProductService(repo, newFakeRegistry())
	bad := "abc"
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{BasePrice: &bad}, false, false); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("err = %v", err)
	}
	active := false
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{IsActive: &active}, false, false); err != nil {
		t.Fatal(err)
	}
}

func TestCreateProductPricingDefaultsAndValidation(t *testing.T) {
	repo := &fakeProducts{}
	svc := NewProductService(repo, newFakeRegistry())
	p, err := svc.Create(context.Background(), scope(t), ProductInput{Name: "Router", BasePrice: "500000"}, "u1")
	if err != nil || p.Pricing != (pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}) {
		t.Fatalf("p=%+v err=%v", p.Pricing, err)
	}
	if _, err := svc.Create(context.Background(), scope(t), ProductInput{Name: "Internet", Pricing: pricing.Attributes{ChargeType: pricing.Recurring}}, "u1"); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("recurring tanpa frekuensi err=%v", err)
	}
	bad := pricing.Attributes{ChargeType: pricing.OneTime, Frequency: pricing.Monthly}
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Pricing: &bad}, false, false); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("update err=%v", err)
	}
	good := pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly}
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Pricing: &good}, false, false); err != nil {
		t.Fatal(err)
	}
	if got := *repo.updated[len(repo.updated)-1].Pricing; got.PaymentTiming != pricing.Prepaid {
		t.Fatalf("update pricing tidak dinormalisasi: %+v", got)
	}
}

func TestCreateProductPlatformOnly(t *testing.T) {
	repo := &fakeProducts{}
	svc := NewProductService(repo, newFakeRegistry())
	base := ProductInput{Name: "A", Unit: "pcs", BasePrice: "1", CategoryID: "c1"}

	in := base
	in.Listing = repository.ListingParams{IsPublic: true, ListingCode: "a"}
	if _, err := svc.Create(context.Background(), scope(t), in, "u"); !errors.Is(err, ErrPlatformOnly) {
		t.Fatalf("listing non-platform err = %v", err)
	}
	in = base
	in.Features = []repository.FeatureValue{{FeatureKey: "crm", Value: json.RawMessage(`true`)}}
	var ce *coreerrors.AppError
	if _, err := svc.Create(context.Background(), scope(t), in, "u"); !errors.As(err, &ce) || ce.Code != "FORBIDDEN" {
		t.Fatalf("features non-platform tanpa permission err = %v", err)
	}
	if _, err := svc.Create(context.Background(), scope(t), base, "u"); err != nil {
		t.Fatalf("tanpa field baru harus sukses: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("repo dipanggil %d kali, want 1", len(repo.created))
	}
	// Update: blok listing/fitur pada non-platform ditolak.
	feats := []repository.FeatureValue{}
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Features: &feats}, false, false); !errors.As(err, &ce) || ce.Code != "FORBIDDEN" {
		t.Fatalf("update non-platform err = %v", err)
	}
}

func TestCreateProductListingRules(t *testing.T) {
	repo := &fakeProducts{}
	svc := NewProductService(repo, newFakeRegistry())
	mk := func(category, code string) ProductInput {
		return ProductInput{Name: "A", Unit: "pcs", BasePrice: "1", CategoryID: category, Platform: true,
			Listing: repository.ListingParams{IsPublic: true, ListingCode: code}}
	}
	for name, in := range map[string]ProductInput{
		"tanpa kategori": mk("", "free"),
		"tanpa kode":     mk("c1", ""),
		"kode berspasi":  mk("c1", "Free Lancer"),
	} {
		if _, err := svc.Create(context.Background(), scope(t), in, "u"); !errors.Is(err, ErrInvalidListing) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := svc.Create(context.Background(), scope(t), mk("c1", "freelancer"), "u"); err != nil {
		t.Fatalf("listing valid: %v", err)
	}
	// Update: kategori dikosongkan pada produk yang publik ditolak.
	cat := "c1"
	repo.existing = &domain.Product{ID: "p1", CategoryID: &cat, IsPublic: true, ListingCode: "freelancer"}
	empty := ""
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{CategoryID: &empty}, true, false); !errors.Is(err, ErrInvalidListing) {
		t.Fatalf("hapus kategori produk publik err = %v", err)
	}
}

func TestCreateProductFeatureRules(t *testing.T) {
	svc := NewProductService(&fakeProducts{}, newFakeRegistry())
	mk := func(f ...repository.FeatureValue) ProductInput {
		return ProductInput{Name: "A", Unit: "pcs", BasePrice: "1", Platform: true, Features: f}
	}
	yes := json.RawMessage(`true`)
	many := make([]repository.FeatureValue, 31)
	for i := range many {
		many[i] = repository.FeatureValue{FeatureKey: "crm", Value: yes}
	}
	cases := map[string]ProductInput{
		"key tidak ada":    mk(repository.FeatureValue{FeatureKey: "nope", Value: yes}),
		"key nonaktif":     mk(repository.FeatureValue{FeatureKey: "old", Value: yes}),
		"31 fitur":         mk(many...),
		"key duplikat":     mk(repository.FeatureValue{FeatureKey: "crm", Value: yes}, repository.FeatureValue{FeatureKey: "crm", Value: yes}),
		"nilai salah tipe": mk(repository.FeatureValue{FeatureKey: "users", Value: json.RawMessage(`"5"`)}),
		"label > 200":      mk(repository.FeatureValue{FeatureKey: "crm", Value: yes, DisplayLabel: stringsRepeat("x", 201)}),
	}
	for name, in := range cases {
		if _, err := svc.Create(context.Background(), scope(t), in, "u"); !errors.Is(err, ErrInvalidFeature) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	ok := mk(repository.FeatureValue{FeatureKey: "crm", Value: yes}, repository.FeatureValue{FeatureKey: "users", Value: json.RawMessage(`5`)})
	if _, err := svc.Create(context.Background(), scope(t), ok, "u"); err != nil {
		t.Fatalf("fitur valid: %v", err)
	}
	// Key dinonaktifkan setelah dipakai: menyimpan ulang dengan key itu ditolak.
	feats := []repository.FeatureValue{{FeatureKey: "old", Value: yes}}
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Features: &feats}, true, false); !errors.Is(err, ErrInvalidFeature) {
		t.Fatalf("update dengan key nonaktif err = %v", err)
	}
	// Update tanpa menyentuh fitur tetap boleh.
	name := "Baru"
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Name: &name}, true, false); err != nil {
		t.Fatalf("update tanpa fitur: %v", err)
	}
}

func stringsRepeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func TestValidateFeatureValue(t *testing.T) {
	def := func(vt string) domain.FeatureDef { return domain.FeatureDef{ValueType: vt} }
	cases := []struct {
		vt, v string
		want  bool
	}{
		{"integer", `5`, true}, {"integer", `5.5`, false}, {"integer", `"5"`, false},
		{"boolean", `true`, true}, {"boolean", `"true"`, false},
		{"string", `""`, false}, {"string", `"` + stringsRepeat("x", 201) + `"`, false}, {"string", `"Prioritas"`, true},
		{"decimal", `0.25`, true}, {"decimal", `"0.25"`, false},
	}
	for _, c := range cases {
		if got := ValidateFeatureValue(def(c.vt), json.RawMessage(c.v)); got != c.want {
			t.Errorf("%s %s = %v, want %v", c.vt, c.v, got, c.want)
		}
	}
}

func TestProductReadsFillLabels(t *testing.T) {
	feats := []domain.ProductFeature{
		{FeatureKey: "crm", Value: json.RawMessage(`true`)},
		{FeatureKey: "users", Value: json.RawMessage(`5`)},
		{FeatureKey: "old", Value: json.RawMessage(`true`)},
		{FeatureKey: "crm2", Value: json.RawMessage(`false`)},
	}
	reg := newFakeRegistry()
	reg.defs["crm2"] = domain.FeatureDef{Key: "crm2", Name: "CRM2", ValueType: "boolean", IsActive: true}
	repo := &fakeProducts{
		existing: &domain.Product{ID: "p1", Features: feats},
		byID:     map[string]domain.Product{"p1": {ID: "p1", Features: append([]domain.ProductFeature(nil), feats...)}},
		listed:   []domain.Product{{ID: "p1", Features: append([]domain.ProductFeature(nil), feats...)}},
	}
	svc := NewProductService(repo, reg)
	want := []string{"CRM", "Jumlah user: 5 user", "Lama", ""}
	check := func(where string, got []domain.ProductFeature) {
		for i, w := range want {
			if got[i].Label != w {
				t.Errorf("%s[%d].Label = %q, want %q", where, i, got[i].Label, w)
			}
		}
	}
	p, err := svc.Get(context.Background(), scope(t), "p1")
	if err != nil {
		t.Fatal(err)
	}
	check("Get", p.Features)
	list, _, _ := svc.List(context.Background(), scope(t), repository.ProductListFilter{})
	check("List", list[0].Features)
	m, _ := svc.FindByIDs(context.Background(), scope(t), []string{"p1"})
	check("FindByIDs", m["p1"].Features)
}

func TestCreate_Tenant_Features(t *testing.T) {
	yes := json.RawMessage(`true`)
	f := func(key string, v json.RawMessage) repository.FeatureValue {
		return repository.FeatureValue{FeatureKey: key, Value: v}
	}
	many := make([]repository.FeatureValue, 31)
	for i := range many {
		many[i] = f("tcrm", yes)
	}
	cases := []struct {
		name     string
		canMng   bool
		platform bool
		feats    []repository.FeatureValue
		want     error
		wantCode string
	}{
		{"non-offerable", true, false, []repository.FeatureValue{f("crm", yes)}, ErrFeatureNotOfferable, ""},
		{"di atas max", true, false, []repository.FeatureValue{f("tusr", json.RawMessage(`11`))}, ErrFeatureAboveMax, ""},
		{"integer nol", true, false, []repository.FeatureValue{f("tusr", json.RawMessage(`0`))}, ErrInvalidFeature, ""},
		{"boolean offerable", true, false, []repository.FeatureValue{f("tcrm", yes), f("tusr", json.RawMessage(`10`))}, nil, ""},
		{"tanpa permission", false, false, []repository.FeatureValue{f("tcrm", yes)}, nil, "FORBIDDEN"},
		{"31 fitur", true, false, many, ErrInvalidFeature, ""},
		{"key duplikat", true, false, []repository.FeatureValue{f("tcrm", yes), f("tcrm", yes)}, ErrInvalidFeature, ""},
		{"platform tidak dibatasi", false, true, []repository.FeatureValue{f("crm", yes), f("users", json.RawMessage(`999`))}, nil, ""},
	}
	for _, c := range cases {
		repo := &fakeProducts{}
		svc := NewProductService(repo, newFakeRegistry())
		in := ProductInput{Name: "A", Unit: "pcs", BasePrice: "1", Platform: c.platform, CanManageEntitlements: c.canMng, Features: c.feats}
		_, err := svc.Create(context.Background(), scope(t), in, "u")
		switch {
		case c.wantCode != "":
			var ce *coreerrors.AppError
			if !errors.As(err, &ce) || ce.Code != c.wantCode || ce.Status != http.StatusForbidden {
				t.Errorf("%s: err = %v", c.name, err)
			}
		case !errors.Is(err, c.want) || (c.want == nil && err != nil):
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if err != nil && len(repo.created) != 0 {
			t.Errorf("%s: repo tidak boleh dipanggil", c.name)
		}
	}
}

func TestCreate_Tenant_ListingStillPlatformOnly(t *testing.T) {
	svc := NewProductService(&fakeProducts{}, newFakeRegistry())
	in := ProductInput{Name: "A", Unit: "pcs", BasePrice: "1", CanManageEntitlements: true, Listing: repository.ListingParams{IsPublic: true, ListingCode: "a"}}
	if _, err := svc.Create(context.Background(), scope(t), in, "u"); !errors.Is(err, ErrPlatformOnly) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdate_Tenant_Features(t *testing.T) {
	svc := NewProductService(&fakeProducts{}, newFakeRegistry())
	ctx := context.Background()
	ok := []repository.FeatureValue{{FeatureKey: "tcrm", Value: json.RawMessage(`true`)}}
	bad := []repository.FeatureValue{{FeatureKey: "crm", Value: json.RawMessage(`true`)}}
	name := "Baru"
	// tanpa fitur: tidak butuh permission
	if _, err := svc.Update(ctx, scope(t), "p1", repository.UpdateProductParams{Name: &name}, false, false); err != nil {
		t.Fatalf("update tanpa fitur: %v", err)
	}
	// kirim fitur tanpa permission -> 403
	var ce *coreerrors.AppError
	if _, err := svc.Update(ctx, scope(t), "p1", repository.UpdateProductParams{Features: &ok}, false, false); !errors.As(err, &ce) || ce.Code != "FORBIDDEN" {
		t.Fatalf("tanpa permission err = %v", err)
	}
	// dengan permission, valid dan tidak valid
	if _, err := svc.Update(ctx, scope(t), "p1", repository.UpdateProductParams{Features: &ok}, false, true); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if _, err := svc.Update(ctx, scope(t), "p1", repository.UpdateProductParams{Features: &bad}, false, true); !errors.Is(err, ErrFeatureNotOfferable) {
		t.Fatalf("non-offerable err = %v", err)
	}
	// listing tetap platform-only
	l := repository.ListingParams{IsPublic: true, ListingCode: "a"}
	if _, err := svc.Update(ctx, scope(t), "p1", repository.UpdateProductParams{Listing: &l}, false, true); !errors.Is(err, ErrPlatformOnly) {
		t.Fatalf("listing err = %v", err)
	}
}

func tenantCtx(t *testing.T, orgType coretenant.OrganizationType) context.Context {
	t.Helper()
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111", OrganizationSlug: "org-a",
		OrganizationType: orgType, OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active", MembershipVersion: 1,
		ResolutionSource: coretenant.ResolutionSourceSession, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	return coretenant.WithContext(context.Background(), tc)
}

type countingRegistry struct {
	*fakeRegistry
	calls int
}

func (c *countingRegistry) FindByKeys(ctx context.Context, keys []string) (map[string]domain.FeatureDef, error) {
	c.calls++
	return c.fakeRegistry.FindByKeys(ctx, keys)
}

func TestProductNeedsUpdateOnReads(t *testing.T) {
	mk := func(id, key, val string) domain.Product {
		return domain.Product{ID: id, Features: []domain.ProductFeature{{FeatureKey: key, Value: json.RawMessage(val)}}}
	}
	build := func() (*fakeProducts, *countingRegistry) {
		reg := &countingRegistry{fakeRegistry: newFakeRegistry()}
		// tusr max=10: 5 muat, 50 melebihi; tcrm offerable; wa tidak offerable.
		all := []domain.Product{mk("ok", "tusr", "5"), mk("over", "tusr", "50"), mk("revoked", "wa", "true"), mk("okbool", "tcrm", "true")}
		byID := map[string]domain.Product{}
		for _, p := range all {
			byID[p.ID] = p
		}
		over := all[1]
		return &fakeProducts{existing: &over, byID: byID, listed: all}, reg
	}
	want := map[string]bool{"ok": false, "over": true, "revoked": true, "okbool": false}

	t.Run("tenant list: satu FindByKeys", func(t *testing.T) {
		repo, reg := build()
		list, _, err := NewProductService(repo, reg).List(tenantCtx(t, coretenant.OrganizationTypeCustomer), scope(t), repository.ProductListFilter{})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range list {
			if p.NeedsUpdate != want[p.ID] {
				t.Errorf("%s NeedsUpdate=%v, want %v", p.ID, p.NeedsUpdate, want[p.ID])
			}
		}
		if reg.calls != 1 {
			t.Errorf("FindByKeys calls = %d, want 1", reg.calls)
		}
	})
	t.Run("tenant get", func(t *testing.T) {
		repo, reg := build()
		p, err := NewProductService(repo, reg).Get(tenantCtx(t, coretenant.OrganizationTypeCustomer), scope(t), "over")
		if err != nil || !p.NeedsUpdate {
			t.Fatalf("p=%+v err=%v", p, err)
		}
	})
	t.Run("platform selalu false", func(t *testing.T) {
		repo, reg := build()
		list, _, _ := NewProductService(repo, reg).List(tenantCtx(t, coretenant.OrganizationTypePlatform), scope(t), repository.ProductListFilter{})
		for _, p := range list {
			if p.NeedsUpdate {
				t.Errorf("%s platform NeedsUpdate=true", p.ID)
			}
		}
	})
}

// Kontrak yang sudah terjual tidak disentuh: service produk hanya punya repo produk
// + registry fitur (tidak ada dependensi writer entitlement), dan jalur baca tidak
// memanggil tulis apa pun pada repo produk.
func TestProductReadsNeverWrite(t *testing.T) {
	over := domain.Product{ID: "over", Features: []domain.ProductFeature{{FeatureKey: "tusr", Value: json.RawMessage(`50`)}}}
	repo := &fakeProducts{existing: &over, byID: map[string]domain.Product{"over": over}, listed: []domain.Product{over}}
	svc := NewProductService(repo, newFakeRegistry())
	ctx := tenantCtx(t, coretenant.OrganizationTypeCustomer)
	_, _ = svc.Get(ctx, scope(t), "over")
	_, _, _ = svc.List(ctx, scope(t), repository.ProductListFilter{})
	_, _ = svc.FindByIDs(ctx, scope(t), []string{"over"})
	if len(repo.created) != 0 || len(repo.updated) != 0 {
		t.Fatalf("jalur baca menulis: created=%d updated=%d", len(repo.created), len(repo.updated))
	}
}
