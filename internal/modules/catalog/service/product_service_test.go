package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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

type fakeRegistry struct{ defs map[string]domain.FeatureDef }

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{defs: map[string]domain.FeatureDef{
		"crm":   {Key: "crm", Name: "CRM", ValueType: "boolean", IsActive: true},
		"users": {Key: "users", Name: "Jumlah user", ValueType: "integer", Unit: "user", IsActive: true},
		"old":   {Key: "old", Name: "Lama", ValueType: "boolean", IsActive: false},
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
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{BasePrice: &bad}, false); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("err = %v", err)
	}
	active := false
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{IsActive: &active}, false); err != nil {
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
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Pricing: &bad}, false); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("update err=%v", err)
	}
	good := pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly}
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Pricing: &good}, false); err != nil {
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
	if _, err := svc.Create(context.Background(), scope(t), in, "u"); !errors.Is(err, ErrPlatformOnly) {
		t.Fatalf("features non-platform err = %v", err)
	}
	if _, err := svc.Create(context.Background(), scope(t), base, "u"); err != nil {
		t.Fatalf("tanpa field baru harus sukses: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("repo dipanggil %d kali, want 1", len(repo.created))
	}
	// Update: blok listing/fitur pada non-platform ditolak.
	feats := []repository.FeatureValue{}
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Features: &feats}, false); !errors.Is(err, ErrPlatformOnly) {
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
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{CategoryID: &empty}, true); !errors.Is(err, ErrInvalidListing) {
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
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Features: &feats}, true); !errors.Is(err, ErrInvalidFeature) {
		t.Fatalf("update dengan key nonaktif err = %v", err)
	}
	// Update tanpa menyentuh fitur tetap boleh.
	name := "Baru"
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{Name: &name}, true); err != nil {
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
