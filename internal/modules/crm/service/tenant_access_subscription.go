package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
)

const (
	SubscriptionFree            = "free"
	SubscriptionAwaitingPayment = "awaiting_payment"
	SubscriptionActive          = "active"
	SubscriptionOverdue         = "overdue"

	subscriptionInvoiceLimit = 12
)

type SubscriptionProduct struct{ Name, SKU, Frequency string }

type SubscriptionInvoice struct {
	Number, Status, Total, PeriodLabel string
	DueDate                            *time.Time
	URL                                string // link publik; "" untuk draft/void
}

type SubscriptionView struct {
	Status          string
	Product         *SubscriptionProduct
	ContractNumber  string
	NextInvoiceDate *time.Time // next_period_start - invoice_lead_days (prabayar)
	OverdueDays     int        // hari sejak due_date invoice overdue tertua
	SuspendInDays   *int       // graceDays + 1 - OverdueDays, minimal 0; nil bila tidak overdue
	Features        []domain.FeatureSnapshot
	Invoices        []SubscriptionInvoice // 12 terbaru, urut tanggal terbit desc
}

type SubscriptionContractItem struct {
	Name, SKU, Frequency string
	Prepaid              bool
	NextPeriodStart      time.Time
	Features             []domain.FeatureSnapshot
}

type SubscriptionContract struct {
	ID, Number, Status string
	InvoiceLeadDays    int
	Items              []SubscriptionContractItem // urut position
}

// SubscriptionReader: pembacaan sempit ke receivable/katalog untuk ringkasan langganan (diadaptasi di internal/app).
type SubscriptionReader interface {
	Contract(ctx context.Context, scope coretenant.Scope, id string) (SubscriptionContract, error)
	InvoicesBySource(ctx context.Context, scope coretenant.Scope, sourceType, sourceID string) ([]BillingInvoice, error)
	InvoicesByContract(ctx context.Context, scope coretenant.Scope, contractID string) ([]BillingInvoice, error)
	// FreeFeatures: fitur paket gratis (produk SKU FREE) untuk workspace tanpa contract.
	FreeFeatures(ctx context.Context, scope coretenant.Scope) ([]domain.FeatureSnapshot, error)
	Link(ctx context.Context, scope coretenant.Scope, invoiceID string) (string, error)
}

// WithSubscription mengaktifkan ringkasan langganan.
func (a *TenantAccess) WithSubscription(r SubscriptionReader) *TenantAccess {
	a.subscription = r
	return a
}

// Subscription merangkum langganan workspace: company tertaut → SO → contract/invoice. Status:
// free (tanpa contract/SO), awaiting_payment (invoice awal belum lunas), overdue, active.
func (a *TenantAccess) Subscription(ctx context.Context, platform coretenant.Scope, tenantOrganizationID string, today time.Time, graceDays int) (SubscriptionView, error) {
	if a.subscription == nil {
		return SubscriptionView{}, errors.New("tenant access: subscription is not configured")
	}
	free := func() (SubscriptionView, error) {
		features, err := a.subscription.FreeFeatures(ctx, platform)
		if err != nil {
			slog.WarnContext(ctx, "subscription: free features unavailable", "error", err)
		}
		return SubscriptionView{Status: SubscriptionFree, Features: features, Invoices: []SubscriptionInvoice{}}, nil
	}

	company, err := a.companies.FindByTenantOrganization(ctx, platform, tenantOrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return free()
	}
	if err != nil {
		return SubscriptionView{}, fmt.Errorf("read linked company: %w", err)
	}
	orders, err := a.companyOrders(ctx, platform, company.ID)
	if err != nil {
		return SubscriptionView{}, err
	}

	var (
		invoices     = map[string]BillingInvoice{}
		contract     *SubscriptionContract
		unpaidFirst  bool // invoice awal SO belum lunas
		contractPaid bool
	)
	for _, so := range orders {
		if so.Status == domain.SalesOrderCancelled {
			continue
		}
		list, err := a.subscription.InvoicesBySource(ctx, platform, string(receivabledomain.SourceSalesOrder), so.ID)
		if err != nil {
			return SubscriptionView{}, fmt.Errorf("read sales order invoices: %w", err)
		}
		for _, inv := range list {
			invoices[inv.ID] = inv
			if inv.ID == so.InitialInvoiceID && (inv.Status == "draft" || inv.Status == "issued") {
				unpaidFirst = true
			}
		}
		if so.ContractID == "" {
			continue
		}
		c, err := a.subscription.Contract(ctx, platform, so.ContractID)
		if err != nil {
			return SubscriptionView{}, fmt.Errorf("read contract: %w", err)
		}
		list, err = a.subscription.InvoicesByContract(ctx, platform, so.ContractID)
		if err != nil {
			return SubscriptionView{}, fmt.Errorf("read contract invoices: %w", err)
		}
		for _, inv := range list {
			invoices[inv.ID] = inv
			if inv.Status == "paid" {
				contractPaid = true
			}
		}
		if receivabledomain.ContractStatus(c.Status) == receivabledomain.ContractActive && contract == nil {
			cc := c
			contract = &cc
		}
	}
	if contract == nil && len(invoices) == 0 {
		return free()
	}

	view := SubscriptionView{Invoices: a.subscriptionInvoices(ctx, platform, invoices)}
	var oldestOverdue *time.Time
	for _, inv := range invoices {
		if inv.Status == "overdue" && inv.DueDate != nil && (oldestOverdue == nil || inv.DueDate.Before(*oldestOverdue)) {
			d := *inv.DueDate
			oldestOverdue = &d
		}
	}
	switch {
	case oldestOverdue != nil:
		view.Status = SubscriptionOverdue
		view.OverdueDays = max(0, int(today.Sub(*oldestOverdue).Hours()/24))
		left := max(0, graceDays+1-view.OverdueDays)
		view.SuspendInDays = &left
	case contract == nil || (unpaidFirst && !contractPaid):
		view.Status = SubscriptionAwaitingPayment
	default:
		view.Status = SubscriptionActive
	}

	if contract == nil {
		// Belum ada contract aktif: tampilkan paket Free sampai pembayaran selesai.
		features, err := a.subscription.FreeFeatures(ctx, platform)
		if err != nil {
			slog.WarnContext(ctx, "subscription: free features unavailable", "error", err)
		}
		view.Features = features
		return view, nil
	}
	view.ContractNumber = contract.Number
	items := make([]ContractAccessItem, 0, len(contract.Items))
	for _, it := range contract.Items {
		items = append(items, ContractAccessItem{Prepaid: it.Prepaid, Features: it.Features})
	}
	view.Features = MergeFeatures(items)
	if len(contract.Items) > 0 {
		// Produk yang ditampilkan: item berulang pertama (item sekali bayar seperti setup tidak mewakili paket).
		first := contract.Items[0]
		for _, it := range contract.Items {
			if !it.NextPeriodStart.IsZero() {
				first = it
				break
			}
		}
		view.Product = &SubscriptionProduct{Name: first.Name, SKU: first.SKU, Frequency: first.Frequency}
	}
	if view.Status == SubscriptionActive {
		for _, it := range contract.Items {
			if !it.Prepaid || it.NextPeriodStart.IsZero() {
				continue
			}
			next := it.NextPeriodStart.AddDate(0, 0, -contract.InvoiceLeadDays)
			if view.NextInvoiceDate == nil || next.Before(*view.NextInvoiceDate) {
				view.NextInvoiceDate = &next
			}
		}
	}
	return view, nil
}

func (a *TenantAccess) companyOrders(ctx context.Context, platform coretenant.Scope, companyID string) ([]domain.SalesOrder, error) {
	const pageSize = 200
	var out []domain.SalesOrder
	for offset := 0; ; offset += pageSize {
		page, total, err := a.orders.List(ctx, platform, repository.SalesOrderListFilter{CompanyID: companyID, Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, fmt.Errorf("list company sales orders: %w", err)
		}
		out = append(out, page...)
		if int64(offset+len(page)) >= total || len(page) == 0 {
			return out, nil
		}
	}
}

// subscriptionInvoices: 12 invoice terbaru (draft disembunyikan) urut tanggal terbit menurun.
func (a *TenantAccess) subscriptionInvoices(ctx context.Context, platform coretenant.Scope, byID map[string]BillingInvoice) []SubscriptionInvoice {
	list := make([]BillingInvoice, 0, len(byID))
	for _, inv := range byID {
		if inv.Status != "draft" {
			list = append(list, inv)
		}
	}
	issued := func(i BillingInvoice) time.Time {
		if i.IssueDate != nil {
			return *i.IssueDate
		}
		return time.Time{}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if !issued(list[i]).Equal(issued(list[j])) {
			return issued(list[i]).After(issued(list[j]))
		}
		return list[i].Number > list[j].Number
	})
	if len(list) > subscriptionInvoiceLimit {
		list = list[:subscriptionInvoiceLimit]
	}
	out := make([]SubscriptionInvoice, 0, len(list))
	for _, inv := range list {
		item := SubscriptionInvoice{Number: inv.Number, Status: inv.Status, Total: inv.GrandTotal, DueDate: inv.DueDate, PeriodLabel: periodLabel(inv.PeriodStart, inv.PeriodEnd)}
		if inv.Status != "void" {
			url, err := a.subscription.Link(ctx, platform, inv.ID)
			if err != nil {
				slog.WarnContext(ctx, "subscription: invoice link unavailable", "invoice_number", inv.Number, "error", err)
			}
			item.URL = url
		}
		out = append(out, item)
	}
	return out
}

var monthsID = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func periodLabel(from, to *time.Time) string {
	if from == nil || to == nil {
		return ""
	}
	f := func(t time.Time) string { return fmt.Sprintf("%d %s %d", t.Day(), monthsID[t.Month()-1], t.Year()) }
	return f(*from) + " – " + f(*to)
}
