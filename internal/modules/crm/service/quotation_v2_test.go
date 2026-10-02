package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type fakeQuotationRepo struct {
	repository.QuotationRepository
	created  []repository.CreateQuotationParams
	replaced []repository.ReplaceQuotationItemsParams
	revised  []repository.CreateQuotationParams
	expired  int
	stored   map[string]domain.Quotation
}

func (f *fakeQuotationRepo) Create(_ context.Context, _ coretenant.Scope, p repository.CreateQuotationParams) (domain.Quotation, error) {
	f.created = append(f.created, p)
	return domain.Quotation{ID: "q-new", QuotationNumber: p.QuotationNumber}, nil
}
func (f *fakeQuotationRepo) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Quotation, error) {
	return f.stored[id], nil
}
func (f *fakeQuotationRepo) ReplaceItems(_ context.Context, _ coretenant.Scope, id string, p repository.ReplaceQuotationItemsParams) (domain.Quotation, error) {
	f.replaced = append(f.replaced, p)
	return domain.Quotation{ID: id}, nil
}
func (f *fakeQuotationRepo) Update(_ context.Context, _ coretenant.Scope, id string, _ repository.UpdateQuotationParams) (domain.Quotation, error) {
	return f.stored[id], nil
}
func (f *fakeQuotationRepo) Revise(_ context.Context, _ coretenant.Scope, _ string, p repository.CreateQuotationParams) (domain.Quotation, error) {
	f.revised = append(f.revised, p)
	return domain.Quotation{ID: "q-rev", QuotationNumber: p.QuotationNumber}, nil
}
func (f *fakeQuotationRepo) ExpireDue(context.Context, coretenant.Scope, time.Time) (int64, error) {
	f.expired++
	return 0, nil
}

type fakeCounter struct{}

func (fakeCounter) NextNumber(context.Context, coretenant.Scope, string) (int, error) { return 7, nil }

type fakeCatalog map[string]catalogdomain.Product

func (f fakeCatalog) FindByIDs(_ context.Context, _ coretenant.Scope, ids []string) (map[string]catalogdomain.Product, error) {
	out := map[string]catalogdomain.Product{}
	for _, id := range ids {
		if p, ok := f[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

type fakeDeals struct {
	repository.DealRepository
	deals map[string]domain.Deal
}

func (f fakeDeals) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Deal, error) {
	d, ok := f.deals[id]
	if !ok {
		return domain.Deal{}, pgx.ErrNoRows
	}
	return d, nil
}

func quotationFixture() (*quotationService, *fakeQuotationRepo) {
	contact, company := "c1", "co1"
	repo := &fakeQuotationRepo{stored: map[string]domain.Quotation{}}
	svc := NewQuotationService(repo, fakeCounter{},
		WithQuotationCatalog(fakeCatalog{
			"p1":    {ID: "p1", Name: "Internet 50 Mbps", SKU: "NET-50", Unit: "bulan", BasePrice: "350000.00", TaxPercent: "11.00", IsActive: true},
			"p-off": {ID: "p-off", Name: "Lama", Unit: "pcs", BasePrice: "1.00", TaxPercent: "0.00", IsActive: false},
		}),
		WithQuotationDeals(fakeDeals{deals: map[string]domain.Deal{"d1": {ID: "d1", ContactID: &contact, CompanyID: &company}}}),
	).(*quotationService)
	return svc, repo
}

func TestCreateFromDealSnapshotsProduct(t *testing.T) {
	svc, repo := quotationFixture()
	_, err := svc.Create(context.Background(), coretenant.Scope{}, CreateQuotationInput{
		DealID: "d1",
		Items:  []QuotationLineInput{{ProductID: "p1", Quantity: "2"}, {Description: "Instalasi", Quantity: "1", UnitPrice: "500000"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := repo.created[0]
	if p.ContactID != "c1" || p.CompanyID != "co1" {
		t.Fatalf("links from deal = %+v", p)
	}
	a := p.Items[0]
	if a.ProductID != "p1" || a.Description != "Internet 50 Mbps" || a.UnitPrice != "350000.00" || a.TaxPercent != "11.00" || a.SKU != "NET-50" || a.Unit != "bulan" {
		t.Fatalf("snapshot = %+v", a)
	}
	if p.QuotationNumber == "" || p.TaxTotal != "77000.00" {
		t.Fatalf("header = %+v", p)
	}
}

func TestCreateAllowsOverridingSnapshotPrice(t *testing.T) {
	svc, repo := quotationFixture()
	_, err := svc.Create(context.Background(), coretenant.Scope{}, CreateQuotationInput{
		DealID: "d1", Items: []QuotationLineInput{{ProductID: "p1", Quantity: "1", UnitPrice: "300000", TaxPercent: "0", Description: "Promo"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := repo.created[0].Items[0]
	if a.UnitPrice != "300000.00" || a.TaxPercent != "0.00" || a.Description != "Promo" {
		t.Fatalf("override = %+v", a)
	}
}

func TestCreateRejectsInactiveProductAndUnknownDeal(t *testing.T) {
	svc, _ := quotationFixture()
	if _, err := svc.Create(context.Background(), coretenant.Scope{}, CreateQuotationInput{DealID: "d1", Items: []QuotationLineInput{{ProductID: "p-off"}}}); !errors.Is(err, ErrProductInactive) {
		t.Fatalf("inactive err = %v", err)
	}
	if _, err := svc.Create(context.Background(), coretenant.Scope{}, CreateQuotationInput{DealID: "nope", Items: []QuotationLineInput{{Description: "x"}}}); !errors.Is(err, ErrQuotationDealNotFound) {
		t.Fatalf("deal err = %v", err)
	}
}

func TestUpdateWithItemsReplacesThem(t *testing.T) {
	svc, repo := quotationFixture()
	repo.stored["q1"] = domain.Quotation{ID: "q1", Status: domain.QuotationStatusDraft}
	_, err := svc.Update(context.Background(), coretenant.Scope{}, "q1", UpdateQuotationInput{
		Items: []QuotationLineInput{{Description: "A", Quantity: "1", UnitPrice: "100", TaxPercent: "11"}},
	})
	if err != nil || len(repo.replaced) != 1 || repo.replaced[0].GrandTotal != "111.00" {
		t.Fatalf("replaced = %+v err=%v", repo.replaced, err)
	}
}

func TestReviseNumbersFromRoot(t *testing.T) {
	svc, repo := quotationFixture()
	repo.stored["r1"] = domain.Quotation{ID: "r1", QuotationNumber: "QUO-2026-0007-R1", RevisionNo: 1, Status: domain.QuotationStatusSent,
		Items: []domain.QuotationItem{{Description: "A", Quantity: "1.00", UnitPrice: "100.00", TaxPercent: "11.00", LineTotal: "100.00"}}}
	if _, err := svc.Revise(context.Background(), coretenant.Scope{}, "r1", "u1"); err != nil {
		t.Fatal(err)
	}
	got := repo.revised[0]
	if got.QuotationNumber != "QUO-2026-0007-R2" || got.RevisionNo != 2 || got.GrandTotal != "111.00" || len(got.Items) != 1 {
		t.Fatalf("revise params = %+v", got)
	}
}

func TestReadsExpireDueQuotations(t *testing.T) {
	svc, repo := quotationFixture()
	repo.stored["q1"] = domain.Quotation{ID: "q1"}
	_, _ = svc.Get(context.Background(), coretenant.Scope{}, "q1")
	if repo.expired != 1 {
		t.Fatalf("ExpireDue calls = %d", repo.expired)
	}
}

func TestRootQuotationNumber(t *testing.T) {
	for in, want := range map[string]string{"QUO-2026-0007": "QUO-2026-0007", "QUO-2026-0007-R1": "QUO-2026-0007", "QUO-2026-0007-R12": "QUO-2026-0007", "PO-R": "PO-R"} {
		if got := rootQuotationNumber(in); got != want {
			t.Errorf("rootQuotationNumber(%q) = %q, want %q", in, got, want)
		}
	}
}
