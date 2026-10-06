package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/pricing"
)

// ssWorld adalah dunia CRM in-memory untuk menguji orkestrator, termasuk injeksi gagal per operasi.
type ssWorld struct {
	calls    []string
	failOnce map[string]bool // "lead.create", "convert", "quote.create", "quote.accept", "so.approve", "so.confirm", "link"
	locked   bool
	subscr   bool
	paid     bool

	products map[string]catalogdomain.Product
	contacts []domain.Contact // contact yang ada di CRM (milik company tertentu)

	company *domain.Company
	deals   []domain.Deal
	quote   *domain.Quotation
	so      *domain.SalesOrder
	seq     int

	lastLead    repository.CreateLeadParams
	lastConvert ConvertLeadParams
	lastQuote   CreateQuotationInput
	lastDraft   repository.SalesOrderDraftFields
	lostReason  string
	linkedURL   string
}

var errBoom = errors.New("boom")

func (w *ssWorld) log(s string) { w.calls = append(w.calls, s) }
func (w *ssWorld) count(prefix string) int {
	n := 0
	for _, c := range w.calls {
		if c == prefix {
			n++
		}
	}
	return n
}
func (w *ssWorld) fail(key string) error {
	if w.failOnce[key] {
		delete(w.failOnce, key)
		return errBoom
	}
	return nil
}

// ---- Locker / Buyers / Subscriptions
func (w *ssWorld) TryLock(context.Context, string) (func(), bool, error) {
	return func() { w.log("unlock") }, !w.locked, nil
}
func (w *ssWorld) Buyer(context.Context, string) (string, string, string, error) {
	return "Rina", "rina@example.com", "0812", nil
}
func (w *ssWorld) WorkspaceName(context.Context, string) (string, error) { return "Studio Rina", nil }
func (w *ssWorld) HasActiveFeatureContract(context.Context, coretenant.Scope, string) (bool, error) {
	return w.subscr, nil
}

// ---- catalog / pipeline
func (w *ssWorld) FindByIDs(_ context.Context, _ coretenant.Scope, ids []string) (map[string]catalogdomain.Product, error) {
	out := map[string]catalogdomain.Product{}
	for _, id := range ids {
		if p, ok := w.products[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}
func (w *ssWorld) FindByID(context.Context, coretenant.Scope, string) (domain.Pipeline, error) {
	return domain.Pipeline{ID: "pipe", Stages: []domain.PipelineStage{
		{ID: "st0", Position: 0, Name: "Renamed A"}, {ID: "st1", Position: 1, Name: "Renamed B"},
		{ID: "won", Position: 2, IsWon: true}, {ID: "lost", Position: 3, IsLost: true}}}, nil
}

// ---- company / contact
func (w *ssWorld) FindByTenantOrganization(context.Context, coretenant.Scope, string) (domain.Company, error) {
	if w.company == nil {
		return domain.Company{}, pgx.ErrNoRows
	}
	return *w.company, nil
}
func (w *ssWorld) List(_ context.Context, _ coretenant.Scope, f repository.ContactListFilter) ([]domain.Contact, int64, error) {
	w.log("contacts.list")
	var out []domain.Contact
	for _, c := range w.contacts {
		if c.CompanyID != nil && *c.CompanyID == f.CompanyID {
			out = append(out, c)
		}
	}
	return out, int64(len(out)), nil
}

// ---- leads
func (w *ssWorld) Create(ctx context.Context, _ coretenant.Scope, p repository.CreateLeadParams) (domain.Lead, error) {
	w.log("lead.create")
	if err := w.fail("lead.create"); err != nil {
		return domain.Lead{}, err
	}
	w.lastLead = p
	return domain.Lead{ID: "lead1"}, nil
}
func (w *ssWorld) Convert(_ context.Context, _ coretenant.Scope, id string, p ConvertLeadParams) (domain.LeadConversionResult, error) {
	w.log("convert")
	if err := w.fail("convert"); err != nil {
		return domain.LeadConversionResult{}, err
	}
	w.lastConvert = p
	if p.Company.Mode == "new" {
		tenant := p.Company.TenantOrganizationID
		w.company = &domain.Company{ID: "co1", Name: p.Company.Name, TenantOrganizationID: &tenant}
	}
	w.seq++
	deal := domain.Deal{ID: fmt.Sprintf("deal%d", w.seq), StageID: p.Deal.StageID, Title: p.Deal.Title, Value: p.Deal.Value, Status: domain.DealStatusOpen}
	w.deals = append(w.deals, deal)
	return domain.LeadConversionResult{Deal: &deal}, nil
}
func (w *ssWorld) Delete(context.Context, coretenant.Scope, string, string) error {
	w.log("lead.delete")
	return nil
}

// ---- deals
func (w *ssWorld) ListOpenByCompanyAndPipeline(context.Context, coretenant.Scope, string, string) ([]domain.Deal, error) {
	var out []domain.Deal
	for _, d := range w.deals {
		if d.Status == domain.DealStatusOpen {
			out = append(out, d)
		}
	}
	return out, nil
}
func (w *ssWorld) Update(_ context.Context, _ coretenant.Scope, id string, p repository.UpdateDealParams) (domain.Deal, error) {
	w.log("deal.update")
	for i := range w.deals {
		if w.deals[i].ID == id {
			w.deals[i].Title, w.deals[i].Value = *p.Title, *p.Value
			return w.deals[i], nil
		}
	}
	return domain.Deal{}, pgx.ErrNoRows
}
func (w *ssWorld) MoveStage(_ context.Context, _ coretenant.Scope, id, stageID, _ string) (domain.Deal, error) {
	w.log("deal.move:" + stageID)
	for i := range w.deals {
		if w.deals[i].ID == id {
			w.deals[i].StageID = stageID
			return w.deals[i], nil
		}
	}
	return domain.Deal{}, pgx.ErrNoRows
}
func (w *ssWorld) CloseLost(_ context.Context, _ coretenant.Scope, id, reason, _ string) (domain.Deal, error) {
	w.log("deal.lost")
	w.lostReason = reason
	for i := range w.deals {
		if w.deals[i].ID == id {
			w.deals[i].Status = domain.DealStatusLost
			return w.deals[i], nil
		}
	}
	return domain.Deal{}, pgx.ErrNoRows
}

// ---- quotations
func (w *ssWorld) FindSelfServeByDeal(_ context.Context, _ coretenant.Scope, dealID string) (domain.Quotation, error) {
	if w.quote == nil || *w.quote.DealID != dealID {
		return domain.Quotation{}, pgx.ErrNoRows
	}
	return *w.quote, nil
}
func (w *ssWorld) createQuotation(in CreateQuotationInput) (domain.Quotation, error) {
	w.log("quote.create")
	if err := w.fail("quote.create"); err != nil {
		return domain.Quotation{}, err
	}
	w.lastQuote = in
	pid := in.Items[0].ProductID
	q := domain.Quotation{ID: "q" + pid, DealID: &in.DealID, Status: domain.QuotationStatusDraft, Channel: in.Channel,
		Items: []domain.QuotationItem{{ProductID: &pid}}}
	w.quote = &q
	return q, nil
}
func (w *ssWorld) AcceptOnline(_ context.Context, _ coretenant.Scope, id, _ string) (domain.Quotation, error) {
	w.log("quote.accept")
	if err := w.fail("quote.accept"); err != nil {
		return domain.Quotation{}, err
	}
	w.quote.Status = domain.QuotationStatusApproved
	return *w.quote, nil
}

// ---- sales orders
func (w *ssWorld) QuotationApproved(_ context.Context, _ coretenant.Scope, q domain.Quotation, _ string) error {
	w.log("so.approve")
	if err := w.fail("so.approve"); err != nil {
		return err
	}
	so := domain.SalesOrder{ID: "so1", SONumber: "SO-1", QuotationID: q.ID, DealID: q.DealID, Status: domain.SalesOrderDraft}
	w.so = &so
	return nil
}
func (w *ssWorld) ListByDeal(context.Context, coretenant.Scope, string) ([]domain.SalesOrder, error) {
	if w.so == nil {
		return nil, nil
	}
	return []domain.SalesOrder{*w.so}, nil
}
func (w *ssWorld) UpdateDraft(_ context.Context, _ coretenant.Scope, _ string, f repository.SalesOrderDraftFields, _ string) (domain.SalesOrder, error) {
	w.log("so.update")
	w.lastDraft = f
	return *w.so, nil
}
func (w *ssWorld) Confirm(_ context.Context, _ coretenant.Scope, _, _ string) (domain.SalesOrder, error) {
	w.log("so.confirm")
	if err := w.fail("so.confirm"); err != nil {
		return domain.SalesOrder{}, err
	}
	w.so.Status, w.so.BillingStatus, w.so.InitialInvoiceID = domain.SalesOrderConfirmed, domain.BillingDone, "inv1"
	return *w.so, nil
}
func (w *ssWorld) RetryBilling(_ context.Context, _ coretenant.Scope, _, _ string) (domain.SalesOrder, error) {
	w.log("so.retry")
	w.so.BillingStatus, w.so.InitialInvoiceID = domain.BillingDone, "inv1"
	return *w.so, nil
}
func (w *ssWorld) CancelUnpaid(_ context.Context, _ coretenant.Scope, _, reason, _ string) (domain.SalesOrder, error) {
	w.log("so.cancel")
	if w.paid {
		return domain.SalesOrder{}, ErrSalesOrderPaid
	}
	w.so.Status = domain.SalesOrderCancelled
	return *w.so, nil
}

// ---- invoices
func (w *ssWorld) Link(context.Context, coretenant.Scope, string, string) (string, time.Time, error) {
	w.log("link")
	if err := w.fail("link"); err != nil {
		return "", time.Time{}, err
	}
	return "https://pay.test/inv1", time.Time{}, nil
}

// quotationsAdapter memetakan Create(CreateQuotationInput) agar ssWorld bisa memenuhi dua interface "Create" sekaligus.
type quotationsAdapter struct{ w *ssWorld }

func (a quotationsAdapter) Create(_ context.Context, _ coretenant.Scope, in CreateQuotationInput) (domain.Quotation, error) {
	return a.w.createQuotation(in)
}
func (a quotationsAdapter) FindSelfServeByDeal(ctx context.Context, s coretenant.Scope, id string) (domain.Quotation, error) {
	return a.w.FindSelfServeByDeal(ctx, s, id)
}
func (a quotationsAdapter) AcceptOnline(ctx context.Context, s coretenant.Scope, id, by string) (domain.Quotation, error) {
	return a.w.AcceptOnline(ctx, s, id, by)
}

func newProduct(id, sku string) catalogdomain.Product {
	return catalogdomain.Product{
		ID: id, SKU: sku, Name: "Freelancer", BasePrice: "100000", IsActive: true, IsPublic: true,
		Pricing:  pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid},
		Features: []catalogdomain.ProductFeature{{FeatureKey: "landing.pages"}},
	}
}

func newSS(t *testing.T) (SelfServeService, *ssWorld) {
	t.Helper()
	w := &ssWorld{failOnce: map[string]bool{}, products: map[string]catalogdomain.Product{
		"p-free": newProduct("p-free", "FREELANCER-M"), "p-biz": newProduct("p-biz", "SMALLBIZ-M"),
	}}
	svc := NewSelfServeService(SelfServeDeps{
		Leads: w, Companies: w, Contacts: w, Deals: w, Pipelines: w, Quotations: quotationsAdapter{w}, Orders: w, Invoices: w,
		Products: w, Buyers: w, Subscriptions: w, Locker: w,
	}, SelfServeConfig{BotUserID: "bot", DealOwnerUserID: "owner", PipelineID: "pipe"},
		func() time.Time { return time.Date(2026, 10, 7, 3, 0, 0, 0, time.UTC) }) // 10:00 WIB
	return svc, w
}

var ssReq = SelfServeCheckoutRequest{TenantOrganizationID: "ws1", BuyerUserID: "u1", ProductID: "p-free"}

func ssScope() coretenant.Scope { return coretenant.Scope{} }

func TestCheckoutFirstPurchaseCreatesFullChain(t *testing.T) {
	svc, w := newSS(t)
	res, err := svc.Checkout(context.Background(), ssScope(), ssReq)
	if err != nil {
		t.Fatal(err)
	}
	if res.InvoiceURL != "https://pay.test/inv1" || res.DealID != "deal1" {
		t.Fatalf("result = %+v", res)
	}
	want := "lead.create,convert,quote.create,quote.accept,so.approve,so.update,so.confirm,link,deal.move:st1,unlock"
	if got := strings.Join(w.calls, ","); got != want {
		t.Fatalf("calls:\n got %s\nwant %s", got, want)
	}
	if !w.lastLead.SkipPlaybook || w.lastLead.Source != "landing_page" || w.lastLead.CreatedBy != "bot" || w.lastLead.OwnerUserID != "owner" {
		t.Fatalf("lead = %+v", w.lastLead)
	}
	cv := w.lastConvert
	if cv.Company.Mode != "new" || cv.Company.Name != "Studio Rina" || cv.Company.TenantOrganizationID != "ws1" || cv.Contact.Mode != "new" {
		t.Fatalf("convert company/contact = %+v %+v", cv.Company, cv.Contact)
	}
	d := cv.Deal
	if d.PipelineID != "pipe" || d.StageID != "st0" || d.Title != "Studio Rina · Freelancer (Bulanan)" || d.Value != "100000" || d.ExpectedCloseDate != "2026-10-07" {
		t.Fatalf("deal = %+v", d)
	}
	q := w.lastQuote
	if q.Channel != domain.QuotationChannelSelfServe || q.DealID != "deal1" || len(q.Items) != 1 || q.Items[0].ProductID != "p-free" || q.Items[0].Quantity != "1" ||
		q.ValidUntil == nil || q.ValidUntil.Format("2006-01-02") != "2026-10-08" || q.Notes != "Pembelian mandiri dari pricing page" || q.CreatedBy != "bot" {
		t.Fatalf("quote = %+v", q)
	}
	f := w.lastDraft
	if f.StartDate.Format("2006-01-02") != "2026-10-07" || f.BillToName != "Rina" || f.BillToCompany != "Studio Rina" || f.BillToEmail != "rina@example.com" ||
		len(f.Channels) != 1 || f.Channels[0] != "email" || f.PICUserID != "bot" {
		t.Fatalf("draft = %+v", f)
	}
}

func TestCheckoutSameProductReturnsSameLink(t *testing.T) {
	svc, w := newSS(t)
	first, err := svc.Checkout(context.Background(), ssScope(), ssReq)
	if err != nil {
		t.Fatal(err)
	}
	w.calls = nil
	second, err := svc.Checkout(context.Background(), ssScope(), ssReq)
	if err != nil || second != first {
		t.Fatalf("second = %+v err=%v, want %+v", second, err, first)
	}
	if got := strings.Join(w.calls, ","); got != "link,unlock" {
		t.Fatalf("calls = %s, want only link", got)
	}
}

func TestCheckoutResumesAfterFailureAtEachStep(t *testing.T) {
	cases := []struct {
		fail, step string
		// panggilan Create yang TIDAK boleh terulang pada percobaan kedua
		notAgain []string
	}{
		{"lead.create", "lead", nil},
		{"convert", "deal", nil},
		{"quote.create", "quotation", []string{"lead.create", "convert"}},
		{"quote.accept", "quotation", []string{"lead.create", "convert", "quote.create"}},
		{"so.approve", "sales_order", []string{"lead.create", "convert", "quote.create", "quote.accept"}},
		{"so.confirm", "sales_order", []string{"lead.create", "convert", "quote.create", "quote.accept", "so.approve"}},
		{"link", "invoice", []string{"lead.create", "convert", "quote.create", "quote.accept", "so.approve", "so.confirm"}},
	}
	for _, c := range cases {
		t.Run(c.fail, func(t *testing.T) {
			svc, w := newSS(t)
			w.failOnce[c.fail] = true
			_, err := svc.Checkout(context.Background(), ssScope(), ssReq)
			var se *SelfServeStepError
			if !errors.As(err, &se) || se.Step != c.step || !errors.Is(err, errBoom) {
				t.Fatalf("err = %v, want step %s", err, c.step)
			}
			w.calls = nil
			res, err := svc.Checkout(context.Background(), ssScope(), ssReq)
			if err != nil || res.InvoiceURL == "" {
				t.Fatalf("retry: %+v err=%v", res, err)
			}
			for _, banned := range c.notAgain {
				if w.count(banned) != 0 {
					t.Errorf("%s repeated on resume: %v", banned, w.calls)
				}
			}
			if len(w.deals) != 1 {
				t.Errorf("deals = %d, want exactly 1", len(w.deals))
			}
		})
	}
}

func TestCheckoutCleansUpLeadWhenConvertFails(t *testing.T) {
	svc, w := newSS(t)
	w.failOnce["convert"] = true
	_, _ = svc.Checkout(context.Background(), ssScope(), ssReq)
	if w.count("lead.delete") != 1 {
		t.Fatalf("orphan lead must be removed: %v", w.calls)
	}
}

func TestCheckoutChangeProductBeforePayment(t *testing.T) {
	svc, w := newSS(t)
	if _, err := svc.Checkout(context.Background(), ssScope(), ssReq); err != nil {
		t.Fatal(err)
	}
	// pembeli sudah punya company+contact; contact lama milik company itu
	w.contacts = []domain.Contact{{ID: "c1", CompanyID: &w.company.ID, Email: "RINA@example.com"}}
	w.calls = nil

	req := ssReq
	req.ProductID = "p-biz"
	res, err := svc.Checkout(context.Background(), ssScope(), req)
	if err != nil || res.DealID != "deal2" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if w.lostReason != "Ganti pilihan ke SMALLBIZ-M" {
		t.Fatalf("lost reason = %q", w.lostReason)
	}
	got := strings.Join(w.calls, ",")
	if !strings.HasPrefix(got, "so.cancel,deal.lost,contacts.list,lead.create,convert,quote.create") {
		t.Fatalf("calls = %s", got)
	}
	if w.lastConvert.Company.Mode != "existing" || w.lastConvert.Company.CompanyID != "co1" ||
		w.lastConvert.Contact.Mode != "existing" || w.lastConvert.Contact.ContactID != "c1" {
		t.Fatalf("company/contact reuse = %+v %+v", w.lastConvert.Company, w.lastConvert.Contact)
	}
}

func TestCheckoutChangeProductAfterPayment(t *testing.T) {
	svc, w := newSS(t)
	if _, err := svc.Checkout(context.Background(), ssScope(), ssReq); err != nil {
		t.Fatal(err)
	}
	w.paid = true
	w.calls = nil
	req := ssReq
	req.ProductID = "p-biz"
	_, err := svc.Checkout(context.Background(), ssScope(), req)
	if !errors.Is(err, ErrSelfServeAlreadySubscribed) {
		t.Fatalf("err = %v", err)
	}
	if w.count("deal.lost") != 0 || w.count("lead.create") != 0 {
		t.Fatalf("nothing may change: %v", w.calls)
	}
}

func TestCheckoutRejectsUnavailableProduct(t *testing.T) {
	mutate := map[string]func(*catalogdomain.Product){
		"one_time": func(p *catalogdomain.Product) {
			p.Pricing = pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}
		},
		"postpaid":     func(p *catalogdomain.Product) { p.Pricing.PaymentTiming = pricing.Postpaid },
		"harga 0":      func(p *catalogdomain.Product) { p.BasePrice = "0" },
		"tanpa fitur":  func(p *catalogdomain.Product) { p.Features = nil },
		"nonaktif":     func(p *catalogdomain.Product) { p.IsActive = false },
		"tidak publik": func(p *catalogdomain.Product) { p.IsPublic = false },
	}
	for name, m := range mutate {
		t.Run(name, func(t *testing.T) {
			svc, w := newSS(t)
			p := w.products["p-free"]
			m(&p)
			w.products["p-free"] = p
			if _, err := svc.Checkout(context.Background(), ssScope(), ssReq); !errors.Is(err, ErrSelfServeProductUnavailable) {
				t.Fatalf("err = %v", err)
			}
			if w.count("lead.create") != 0 {
				t.Fatal("nothing may be created")
			}
		})
	}
	t.Run("tidak ada di katalog", func(t *testing.T) {
		svc, _ := newSS(t)
		req := ssReq
		req.ProductID = "ghost"
		if _, err := svc.Checkout(context.Background(), ssScope(), req); !errors.Is(err, ErrSelfServeProductUnavailable) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCheckoutAlreadySubscribed(t *testing.T) {
	svc, w := newSS(t)
	w.company = &domain.Company{ID: "co1", Name: "Studio Rina"}
	w.subscr = true
	if _, err := svc.Checkout(context.Background(), ssScope(), ssReq); !errors.Is(err, ErrSelfServeAlreadySubscribed) {
		t.Fatalf("err = %v", err)
	}
	if w.count("lead.create") != 0 {
		t.Fatal("no lead may be created")
	}
}

func TestCheckoutLocked(t *testing.T) {
	svc, w := newSS(t)
	w.locked = true
	if _, err := svc.Checkout(context.Background(), ssScope(), ssReq); !errors.Is(err, ErrSelfServeInProgress) {
		t.Fatalf("err = %v", err)
	}
	if len(w.calls) != 0 {
		t.Fatalf("calls = %v", w.calls)
	}
}

// Review Focus 2 & 3: email pembeli sudah ada sebagai contact milik company lain, dan nama workspace
// sama dengan company yang tidak tertaut → company & contact BARU; tidak ada pencarian di luar company tertaut.
func TestCheckoutNeverMergesWithUnlinkedRecords(t *testing.T) {
	svc, w := newSS(t)
	other := "co-other"
	w.contacts = []domain.Contact{{ID: "c-old", CompanyID: &other, Email: "rina@example.com"}}
	if _, err := svc.Checkout(context.Background(), ssScope(), ssReq); err != nil {
		t.Fatal(err)
	}
	if w.lastConvert.Company.Mode != "new" || w.lastConvert.Contact.Mode != "new" {
		t.Fatalf("must create new company and contact: %+v %+v", w.lastConvert.Company, w.lastConvert.Contact)
	}
	if w.count("contacts.list") != 0 {
		t.Fatal("contact lookup must not run without a linked company")
	}
}

// Review Focus 5.
func TestCheckoutProductDeactivatedBetweenClicks(t *testing.T) {
	svc, w := newSS(t)
	first, err := svc.Checkout(context.Background(), ssScope(), ssReq)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"p-free", "p-biz"} {
		p := w.products[id]
		p.IsActive = false
		w.products[id] = p
	}
	again, err := svc.Checkout(context.Background(), ssScope(), ssReq)
	if err != nil || again != first {
		t.Fatalf("same choice must keep its link: %+v err=%v", again, err)
	}
	w.calls = nil
	req := ssReq
	req.ProductID = "p-biz"
	if _, err := svc.Checkout(context.Background(), ssScope(), req); !errors.Is(err, ErrSelfServeProductUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if w.count("so.cancel") != 0 || w.count("deal.lost") != 0 {
		t.Fatalf("old deal must stay: %v", w.calls)
	}
}
