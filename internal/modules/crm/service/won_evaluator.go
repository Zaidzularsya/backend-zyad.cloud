package service

import (
	"context"
	"fmt"
	"log/slog"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/pricing"
)

const (
	WonKindPrepaidOneTime    = "prepaid_one_time"
	WonKindPrepaidRecurring  = "prepaid_recurring"
	WonKindPostpaidRecurring = "postpaid_recurring"
	WonKindPostpaidOneTime   = "postpaid_one_time"
)

type WonCondition struct {
	ItemID, Description string
	Kind                string
	Met                 bool
	Label               string // "Invoice INV-… lunas" | "Menunggu pembayaran INV-…" | "Kontrak CTR-… terbentuk" | "Menunggu konfirmasi diterima"
}

type WonChecklist struct {
	SalesOrderID, SONumber string
	Conditions             []WonCondition
	AllMet                 bool
}

type DealCloser interface {
	CloseWon(ctx context.Context, scope coretenant.Scope, id, updatedBy string) (domain.Deal, error)
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Deal, error)
}

// WonEvaluator menentukan Won otomatis (R18): deal open menjadi won bila semua baris salah satu SO
// confirmed/completed sudah memenuhi syaratnya. Aman dipanggil berulang.
type WonEvaluator struct {
	orders     repository.SalesOrderRepository
	deals      DealCloser
	invoices   InvoiceStatusReader
	contracts  ContractReader
	activities repository.ActivityRepository
}

func NewWonEvaluator(orders repository.SalesOrderRepository, deals DealCloser, invoices InvoiceStatusReader,
	contracts ContractReader, activities repository.ActivityRepository) *WonEvaluator {
	return &WonEvaluator{orders: orders, deals: deals, invoices: invoices, contracts: contracts, activities: activities}
}

// Checklist: per SO confirmed/completed milik deal (urutan repository: terbaru dulu).
func (w *WonEvaluator) Checklist(ctx context.Context, scope coretenant.Scope, dealID string) ([]WonChecklist, error) {
	orders, err := w.orders.ListByDeal(ctx, scope, dealID)
	if err != nil {
		return nil, err
	}
	out := []WonChecklist{}
	for _, so := range orders {
		if so.Status != domain.SalesOrderConfirmed && so.Status != domain.SalesOrderCompleted {
			continue
		}
		out = append(out, w.checklistFor(ctx, scope, so))
	}
	return out, nil
}

func (w *WonEvaluator) checklistFor(ctx context.Context, scope coretenant.Scope, so domain.SalesOrder) WonChecklist {
	cl := WonChecklist{SalesOrderID: so.ID, SONumber: so.SONumber, Conditions: []WonCondition{}}
	invStatus, invNumber := "", ""
	if so.InitialInvoiceID != "" && w.invoices != nil {
		if st, num, err := w.invoices.InvoiceStatus(ctx, scope, so.InitialInvoiceID); err == nil {
			invStatus, invNumber = st, num
		} else {
			slog.WarnContext(ctx, "won checklist: invoice status failed", "invoice_id", so.InitialInvoiceID, "error", err)
		}
	}
	paidLabel := func() (bool, string) {
		switch {
		case invStatus == "paid":
			return true, fmt.Sprintf("Invoice %s lunas", invNumber)
		case invNumber != "":
			return false, fmt.Sprintf("Menunggu pembayaran %s", invNumber)
		default:
			return false, "Menunggu penagihan"
		}
	}
	for _, it := range so.Items {
		c := WonCondition{ItemID: it.ID, Description: it.Description}
		prepaid := it.Pricing.PaymentTiming != pricing.Postpaid
		recurring := it.Pricing.ChargeType == pricing.Recurring
		switch {
		case !recurring && prepaid:
			c.Kind = WonKindPrepaidOneTime
			c.Met, c.Label = paidLabel()
		case recurring && prepaid:
			c.Kind = WonKindPrepaidRecurring
			c.Met, c.Label = paidLabel()
		case recurring:
			c.Kind = WonKindPostpaidRecurring
			if so.ContractID != "" {
				c.Met = true
				c.Label = "Kontrak terbentuk"
				if w.contracts != nil {
					if num, err := w.contracts.ContractNumber(ctx, scope, so.ContractID); err == nil && num != "" {
						c.Label = fmt.Sprintf("Kontrak %s terbentuk", num)
					}
				}
			} else {
				c.Label = "Menunggu kontrak terbentuk"
			}
		default:
			c.Kind = WonKindPostpaidOneTime
			c.Met = it.DeliveryStatus == domain.DeliveryDelivered
			c.Label = "Menunggu konfirmasi diterima"
			if c.Met {
				c.Label = "Sudah diterima"
			}
		}
		cl.Conditions = append(cl.Conditions, c)
	}
	cl.AllMet = len(cl.Conditions) > 0
	for _, c := range cl.Conditions {
		if !c.Met {
			cl.AllMet = false
		}
	}
	return cl
}

// Evaluate: deal open + ada SO dengan AllMet → CloseWon (tanpa aktor) + aktivitas. Deal won/lost tidak disentuh.
func (w *WonEvaluator) Evaluate(ctx context.Context, scope coretenant.Scope, dealID string) (bool, error) {
	deal, err := w.deals.Get(ctx, scope, dealID)
	if err != nil {
		return false, err
	}
	if deal.Status != domain.DealStatusOpen {
		return false, nil
	}
	lists, err := w.Checklist(ctx, scope, dealID)
	if err != nil {
		return false, err
	}
	for _, cl := range lists {
		if !cl.AllMet {
			continue
		}
		if _, err := w.deals.CloseWon(ctx, scope, dealID, ""); err != nil {
			return false, err
		}
		if w.activities != nil {
			if _, err := w.activities.Create(ctx, scope, repository.CreateActivityParams{
				RelatedEntityType: domain.ActivityEntityDeal, RelatedEntityID: dealID, Type: domain.ActivityTypeOrder,
				Subject: "Won otomatis — semua syarat terpenuhi", Status: domain.ActivityStatusCompleted,
			}); err != nil {
				slog.WarnContext(ctx, "won activity failed", "deal_id", dealID, "error", err)
			}
		}
		return true, nil
	}
	return false, nil
}
