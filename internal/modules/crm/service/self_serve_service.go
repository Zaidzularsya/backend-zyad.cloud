package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/pricing"
)

const (
	selfServeLeadSource = "landing_page"
	selfServeQuoteNote  = "Pembelian mandiri dari pricing page"
)

type selfServeService struct {
	deps SelfServeDeps
	cfg  SelfServeConfig
	now  func() time.Time
}

func NewSelfServeService(deps SelfServeDeps, cfg SelfServeConfig, now func() time.Time) SelfServeService {
	if now == nil {
		now = time.Now
	}
	return &selfServeService{deps: deps, cfg: cfg, now: now}
}

func stepErr(step string, err error) error {
	if err == nil {
		return nil
	}
	return &SelfServeStepError{Step: step, Err: err}
}

// chain adalah konteks satu pemanggilan Checkout.
type chain struct {
	platform coretenant.Scope
	req      SelfServeCheckoutRequest
	product  *catalogdomain.Product // nil bila tidak ada di katalog
	stage0   string                 // "Checkout dimulai"
	stage1   string                 // "Menunggu pembayaran"
}

func (s *selfServeService) Checkout(ctx context.Context, platform coretenant.Scope, req SelfServeCheckoutRequest) (SelfServeCheckoutResult, error) {
	unlock, ok, err := s.deps.Locker.TryLock(ctx, "self_serve:"+req.TenantOrganizationID)
	if err != nil {
		return SelfServeCheckoutResult{}, err
	}
	if !ok {
		return SelfServeCheckoutResult{}, ErrSelfServeInProgress
	}
	defer unlock()

	c := &chain{platform: platform, req: req}
	products, err := s.deps.Products.FindByIDs(ctx, platform, []string{req.ProductID})
	if err != nil {
		return SelfServeCheckoutResult{}, err
	}
	if p, found := products[req.ProductID]; found {
		c.product = &p
	}
	if err := s.resolveStages(ctx, c); err != nil {
		return SelfServeCheckoutResult{}, stepErr(SelfServeStepDeal, err)
	}

	company, err := s.deps.Companies.FindByTenantOrganization(ctx, platform, req.TenantOrganizationID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		company = domain.Company{}
	case err != nil:
		return SelfServeCheckoutResult{}, stepErr(SelfServeStepCompany, err)
	}

	var deal *domain.Deal
	var quote *domain.Quotation
	if company.ID != "" {
		deal, quote, err = s.resumeOrReplaceDeal(ctx, c, company)
		if err != nil {
			return SelfServeCheckoutResult{}, err
		}
	}
	if deal == nil {
		deal, err = s.startChain(ctx, c, company)
		if err != nil {
			return SelfServeCheckoutResult{}, err
		}
	}
	return s.finish(ctx, c, *deal, quote)
}

// resolveStages mengambil stage pertama dan kedua pipeline lewat posisi (nama bisa diganti admin).
func (s *selfServeService) resolveStages(ctx context.Context, c *chain) error {
	p, err := s.deps.Pipelines.FindByID(ctx, c.platform, s.cfg.PipelineID)
	if err != nil {
		return err
	}
	for _, st := range p.Stages {
		switch st.Position {
		case 0:
			c.stage0 = st.ID
		case 1:
			c.stage1 = st.ID
		}
	}
	if c.stage0 == "" || c.stage1 == "" {
		return errors.New("self-serve pipeline must have stages at position 0 and 1")
	}
	return nil
}

// resumeOrReplaceDeal memeriksa deal open company di pipeline Self-Serve. Mengembalikan deal yang
// dilanjutkan (dan quote-nya bila ada), atau nil bila rantai baru harus dibuat (pilihan diganti).
func (s *selfServeService) resumeOrReplaceDeal(ctx context.Context, c *chain, company domain.Company) (*domain.Deal, *domain.Quotation, error) {
	deals, err := s.deps.Deals.ListOpenByCompanyAndPipeline(ctx, c.platform, company.ID, s.cfg.PipelineID)
	if err != nil {
		return nil, nil, stepErr(SelfServeStepDeal, err)
	}
	if len(deals) == 0 {
		return nil, nil, nil
	}
	deal := deals[0]
	quote, err := s.deps.Quotations.FindSelfServeByDeal(ctx, c.platform, deal.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		quote = domain.Quotation{}
	case err != nil:
		return nil, nil, stepErr(SelfServeStepQuotation, err)
	}

	if quote.ID != "" && quoteProductID(quote) == c.req.ProductID {
		return &deal, &quote, nil // pilihan sama: lanjutkan
	}
	// Produk baru harus valid SEBELUM ada yang dibatalkan.
	if err := validateSelfServeProduct(c.product); err != nil {
		return nil, nil, err
	}
	if quote.ID == "" {
		// Deal ada tetapi quote belum terbentuk: lanjutkan dengan produk yang diminta.
		deal, err := s.alignDeal(ctx, c, deal, company)
		if err != nil {
			return nil, nil, err
		}
		return &deal, nil, nil
	}

	// Ganti pilihan (K14): batalkan SO yang belum dibayar, tutup deal lama sebagai Lost.
	reason := "Ganti pilihan ke " + c.product.SKU
	orders, err := s.deps.Orders.ListByDeal(ctx, c.platform, deal.ID)
	if err != nil {
		return nil, nil, stepErr(SelfServeStepSalesOrder, err)
	}
	for _, so := range orders {
		if so.QuotationID != quote.ID {
			continue
		}
		if _, err := s.deps.Orders.CancelUnpaid(ctx, c.platform, so.ID, reason, s.cfg.BotUserID); err != nil {
			if errors.Is(err, ErrSalesOrderPaid) {
				return nil, nil, ErrSelfServeAlreadySubscribed
			}
			return nil, nil, stepErr(SelfServeStepSalesOrder, err)
		}
	}
	if _, err := s.deps.Deals.CloseLost(ctx, c.platform, deal.ID, reason, s.cfg.BotUserID); err != nil {
		return nil, nil, stepErr(SelfServeStepDeal, err)
	}
	return nil, nil, nil
}

// alignDeal menyelaraskan judul/nilai deal yang belum punya quote dengan produk yang diminta.
func (s *selfServeService) alignDeal(ctx context.Context, c *chain, deal domain.Deal, company domain.Company) (domain.Deal, error) {
	title := dealTitle(company.Name, *c.product)
	value := c.product.BasePrice
	if deal.Title == title && deal.Value == value {
		return deal, nil
	}
	updated, err := s.deps.Deals.Update(ctx, c.platform, deal.ID, repository.UpdateDealParams{Title: &title, Value: &value, UpdatedBy: s.cfg.BotUserID})
	if err != nil {
		return domain.Deal{}, stepErr(SelfServeStepDeal, err)
	}
	return updated, nil
}

// startChain membuat lead → company/contact → deal untuk pembeli yang belum punya deal open.
func (s *selfServeService) startChain(ctx context.Context, c *chain, company domain.Company) (*domain.Deal, error) {
	if err := validateSelfServeProduct(c.product); err != nil {
		return nil, err
	}
	if company.ID != "" {
		subscribed, err := s.deps.Subscriptions.HasActiveFeatureContract(ctx, c.platform, company.ID)
		if err != nil {
			return nil, stepErr(SelfServeStepCompany, err)
		}
		if subscribed {
			return nil, ErrSelfServeAlreadySubscribed
		}
	}
	buyerName, buyerEmail, buyerPhone, err := s.deps.Buyers.Buyer(ctx, c.req.BuyerUserID)
	if err != nil {
		return nil, stepErr(SelfServeStepCompany, err)
	}
	workspace, err := s.deps.Buyers.WorkspaceName(ctx, c.req.TenantOrganizationID)
	if err != nil {
		return nil, stepErr(SelfServeStepCompany, err)
	}

	// Company: tertaut → pakai; belum ada → buat baru dan tautkan atomik. Tidak ada pencarian
	// company/contact berdasarkan nama/email di luar company yang tertaut (tidak mencampur pelanggan).
	companyInput := &ConvertCompanyInput{Mode: "new", Name: workspace, TenantOrganizationID: c.req.TenantOrganizationID}
	contactInput := &ConvertContactInput{Mode: "new"}
	if company.ID != "" {
		companyInput = &ConvertCompanyInput{Mode: "existing", CompanyID: company.ID}
		workspace = company.Name
		contactID, err := s.findCompanyContact(ctx, c.platform, company.ID, buyerEmail)
		if err != nil {
			return nil, stepErr(SelfServeStepCompany, err)
		}
		if contactID != "" {
			contactInput = &ConvertContactInput{Mode: "existing", ContactID: contactID}
		}
	}

	lead, err := s.deps.Leads.Create(ctx, c.platform, repository.CreateLeadParams{
		ContactName: buyerName, CompanyName: workspace, Email: buyerEmail, Phone: buyerPhone,
		Source: selfServeLeadSource, OwnerUserID: s.cfg.DealOwnerUserID, CreatedBy: s.cfg.BotUserID, SkipPlaybook: true,
	})
	if err != nil {
		return nil, stepErr(SelfServeStepLead, err)
	}
	result, err := s.deps.Leads.Convert(ctx, c.platform, lead.ID, ConvertLeadParams{
		Company: companyInput, Contact: contactInput, OwnerUserID: s.cfg.DealOwnerUserID, ConvertedBy: s.cfg.BotUserID,
		Deal: &ConvertDealInput{
			PipelineID: s.cfg.PipelineID, StageID: c.stage0, Title: dealTitle(workspace, *c.product),
			Value: c.product.BasePrice, ExpectedCloseDate: businesstime.DayOf(s.now()).Format("2006-01-02"),
			OwnerUserID: s.cfg.DealOwnerUserID,
		},
	})
	if err != nil {
		// Lead yatim tidak boleh menumpuk di CRM; best effort.
		if delErr := s.deps.Leads.Delete(ctx, c.platform, lead.ID, s.cfg.BotUserID); delErr != nil {
			slog.WarnContext(ctx, "self-serve: cleanup lead failed", "lead_id", lead.ID, "error", delErr)
		}
		return nil, stepErr(SelfServeStepDeal, err)
	}
	if result.Deal == nil {
		return nil, stepErr(SelfServeStepDeal, errors.New("convert returned no deal"))
	}
	return result.Deal, nil
}

func (s *selfServeService) findCompanyContact(ctx context.Context, scope coretenant.Scope, companyID, email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", nil
	}
	contacts, _, err := s.deps.Contacts.List(ctx, scope, repository.ContactListFilter{CompanyID: companyID, Search: email, Limit: 20})
	if err != nil {
		return "", err
	}
	for _, ct := range contacts {
		if strings.EqualFold(ct.Email, email) {
			return ct.ID, nil
		}
	}
	return "", nil
}

// finish menyelesaikan quote → SO → invoice dan mengembalikan link pembayaran.
func (s *selfServeService) finish(ctx context.Context, c *chain, deal domain.Deal, quote *domain.Quotation) (SelfServeCheckoutResult, error) {
	bot := s.cfg.BotUserID
	if quote == nil {
		found, err := s.deps.Quotations.FindSelfServeByDeal(ctx, c.platform, deal.ID)
		switch {
		case err == nil:
			quote = &found
		case errors.Is(err, pgx.ErrNoRows):
			created, err := s.createQuote(ctx, c, deal)
			if err != nil {
				return SelfServeCheckoutResult{}, stepErr(SelfServeStepQuotation, err)
			}
			quote = &created
		default:
			return SelfServeCheckoutResult{}, stepErr(SelfServeStepQuotation, err)
		}
	}
	if quote.Status == domain.QuotationStatusDraft {
		accepted, err := s.deps.Quotations.AcceptOnline(ctx, c.platform, quote.ID, bot)
		if err != nil {
			return SelfServeCheckoutResult{}, stepErr(SelfServeStepQuotation, err)
		}
		quote = &accepted
	}

	so, err := s.findOrder(ctx, c, deal.ID, *quote)
	if err != nil {
		return SelfServeCheckoutResult{}, stepErr(SelfServeStepSalesOrder, err)
	}
	if so == nil {
		if err := s.deps.Orders.QuotationApproved(ctx, c.platform, *quote, bot); err != nil {
			return SelfServeCheckoutResult{}, stepErr(SelfServeStepSalesOrder, err)
		}
		if so, err = s.findOrder(ctx, c, deal.ID, *quote); err != nil || so == nil {
			if err == nil {
				err = errors.New("sales order was not created from the approved quotation")
			}
			return SelfServeCheckoutResult{}, stepErr(SelfServeStepSalesOrder, err)
		}
	}
	confirmed, err := s.confirmOrder(ctx, c, *so)
	if err != nil {
		return SelfServeCheckoutResult{}, stepErr(SelfServeStepSalesOrder, err)
	}
	if confirmed.InitialInvoiceID == "" || confirmed.BillingStatus != domain.BillingDone {
		return SelfServeCheckoutResult{}, stepErr(SelfServeStepInvoice, ErrSelfServeBillingNotReady)
	}
	url, _, err := s.deps.Invoices.Link(ctx, c.platform, confirmed.InitialInvoiceID, bot)
	if err != nil {
		return SelfServeCheckoutResult{}, stepErr(SelfServeStepInvoice, err)
	}
	if deal.StageID == c.stage0 {
		// Best effort: link sudah siap, pembeli tidak boleh tertahan karena stage; klik berikutnya mencoba lagi.
		if _, err := s.deps.Deals.MoveStage(ctx, c.platform, deal.ID, c.stage1, bot); err != nil {
			slog.WarnContext(ctx, "self-serve: move deal stage failed", "deal_id", deal.ID, "error", err)
		}
	}
	return SelfServeCheckoutResult{InvoiceURL: url, DealID: deal.ID}, nil
}

func (s *selfServeService) createQuote(ctx context.Context, c *chain, deal domain.Deal) (domain.Quotation, error) {
	validUntil := businesstime.DayOf(s.now()).AddDate(0, 0, 1)
	return s.deps.Quotations.Create(ctx, c.platform, CreateQuotationInput{
		DealID: deal.ID, ValidUntil: &validUntil, Notes: selfServeQuoteNote, Channel: domain.QuotationChannelSelfServe,
		Items:     []QuotationLineInput{{ProductID: c.req.ProductID, Quantity: "1"}},
		CreatedBy: s.cfg.BotUserID,
	})
}

func (s *selfServeService) findOrder(ctx context.Context, c *chain, dealID string, q domain.Quotation) (*domain.SalesOrder, error) {
	orders, err := s.deps.Orders.ListByDeal(ctx, c.platform, dealID)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		if orders[i].QuotationID == q.ID {
			return &orders[i], nil
		}
	}
	return nil, nil
}

// confirmOrder membawa SO ke confirmed + invoice awal terbit, sesuai status SO saat ini.
func (s *selfServeService) confirmOrder(ctx context.Context, c *chain, so domain.SalesOrder) (domain.SalesOrder, error) {
	bot := s.cfg.BotUserID
	switch so.Status {
	case domain.SalesOrderDraft:
		name, email, phone, err := s.deps.Buyers.Buyer(ctx, c.req.BuyerUserID)
		if err != nil {
			return domain.SalesOrder{}, err
		}
		workspace, err := s.deps.Buyers.WorkspaceName(ctx, c.req.TenantOrganizationID)
		if err != nil {
			return domain.SalesOrder{}, err
		}
		today := businesstime.DayOf(s.now())
		if _, err := s.deps.Orders.UpdateDraft(ctx, c.platform, so.ID, repository.SalesOrderDraftFields{
			StartDate: &today, BillToName: name, BillToCompany: workspace, BillToEmail: email, BillToPhone: phone,
			Channels: []string{"email"}, PICUserID: bot,
		}, bot); err != nil {
			return domain.SalesOrder{}, err
		}
		return s.deps.Orders.Confirm(ctx, c.platform, so.ID, bot)
	case domain.SalesOrderConfirmed, domain.SalesOrderCompleted:
		if so.BillingStatus == domain.BillingFailed || so.BillingStatus == domain.BillingPending {
			return s.deps.Orders.RetryBilling(ctx, c.platform, so.ID, bot)
		}
		return so, nil
	default:
		return domain.SalesOrder{}, fmt.Errorf("sales order %s is %s", so.SONumber, so.Status)
	}
}

func quoteProductID(q domain.Quotation) string {
	if len(q.Items) == 0 || q.Items[0].ProductID == nil {
		return ""
	}
	return *q.Items[0].ProductID
}

func dealTitle(workspace string, p catalogdomain.Product) string {
	return fmt.Sprintf("%s · %s (%s)", workspace, p.Name, p.Pricing.Frequency.Label())
}

// validateSelfServeProduct menerapkan R1: publik, aktif, langganan prabayar, berharga, dan berfitur.
func validateSelfServeProduct(p *catalogdomain.Product) error {
	if p == nil || p.DeletedAt != nil || !p.IsActive || !p.IsPublic ||
		p.Pricing.ChargeType != pricing.Recurring || p.Pricing.PaymentTiming != pricing.Prepaid || len(p.Features) == 0 {
		return ErrSelfServeProductUnavailable
	}
	price, ok := new(big.Rat).SetString(p.BasePrice)
	if !ok || price.Sign() <= 0 {
		return ErrSelfServeProductUnavailable
	}
	return nil
}
