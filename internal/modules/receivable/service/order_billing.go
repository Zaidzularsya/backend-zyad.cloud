package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/pricing"
)

type OrderLine struct {
	SourceLineID, Description, Quantity, Unit, UnitPrice, DiscountPercent, TaxPercent, ProductID, SKU string
	Attrs                                                                                             pricing.Attributes
	// Features: array JSON [{feature_key,value,label}] yang tidak ditafsirkan receivable; nil disimpan sebagai [].
	Features json.RawMessage
}

type OrderBillingRequest struct {
	SourceType                              domain.SourceType // "sales_order"
	SourceID                                string
	Account                                 AccountInput
	StartDate                               time.Time
	Channels                                []string
	PICUserID, Currency, Notes, ActorUserID string
	Lines                                   []OrderLine
}

type OrderBillingResult struct{ AccountID, InitialInvoiceID, ContractID string }

// OrderBilling menagih sebuah order (mis. Sales Order) dan aman diulang: setiap langkah
// idempoten berdasarkan (SourceType, SourceID).
type OrderBilling interface {
	// BillOrder: account → contract (bila ada baris berulang) → invoice awal (bila ada baris prabayar).
	BillOrder(ctx context.Context, scope coretenant.Scope, req OrderBillingRequest) (OrderBillingResult, error)
	// BillDelivery: satu invoice (idempotency_key "delivery:"+batchKey) untuk baris one_time postpaid yang dikonfirmasi.
	BillDelivery(ctx context.Context, scope coretenant.Scope, sourceType domain.SourceType, sourceID, batchKey string, lines []OrderLine, req OrderBillingRequest) (invoiceID string, err error)
}

type orderBilling struct {
	accounts  repository.AccountRepository
	invoices  InvoiceService
	contracts repository.ContractRepository
	counters  repository.CounterRepository
	registry  *Registry
	now       func() time.Time
}

func NewOrderBilling(accounts repository.AccountRepository, invoices InvoiceService, contracts repository.ContractRepository,
	counters repository.CounterRepository, registry *Registry) OrderBilling {
	return &orderBilling{accounts: accounts, invoices: invoices, contracts: contracts, counters: counters, registry: registry, now: time.Now}
}

var indoMonths = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func indoDate(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), indoMonths[t.Month()-1], t.Year())
}

func (l OrderLine) attrs() (pricing.Attributes, error) { return pricing.Normalize(l.Attrs) }

func (l OrderLine) lineInput(attrs pricing.Attributes) LineInput {
	return LineInput{
		LineInput: pricing.LineInput{Description: l.Description, Quantity: l.Quantity, UnitPrice: l.UnitPrice,
			DiscountPercent: l.DiscountPercent, TaxPercent: l.TaxPercent, Attrs: attrs},
		Unit: l.Unit, ProductID: l.ProductID, SKU: l.SKU, SourceLineID: l.SourceLineID,
	}
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

type classifiedLine struct {
	line         OrderLine
	attrs        pricing.Attributes
	recurringIdx int // posisi di daftar baris berulang (= posisi item kontrak); -1 bila sekali bayar
}

func classify(lines []OrderLine) (prepaid, recurring []classifiedLine, err error) {
	for _, l := range lines {
		attrs, err := l.attrs()
		if err != nil {
			return nil, nil, invalidInvoice("Atribut harga baris \"%s\" tidak valid.", l.Description)
		}
		c := classifiedLine{line: l, attrs: attrs, recurringIdx: -1}
		if attrs.ChargeType == pricing.Recurring {
			c.recurringIdx = len(recurring)
			recurring = append(recurring, c)
		}
		if attrs.PaymentTiming == pricing.Prepaid {
			prepaid = append(prepaid, c)
		}
	}
	return prepaid, recurring, nil
}

func (b *orderBilling) BillOrder(ctx context.Context, scope coretenant.Scope, req OrderBillingRequest) (OrderBillingResult, error) {
	if req.SourceID == "" || req.StartDate.IsZero() {
		return OrderBillingResult{}, invalidInvoice("Sumber order dan tanggal mulai wajib diisi.")
	}
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = domain.SourceSalesOrder
	}
	start := pricing.AddPeriod(req.StartDate, pricing.Daily, 0) // normalisasi ke tanggal kalender UTC
	prepaid, recurring, err := classify(req.Lines)
	if err != nil {
		return OrderBillingResult{}, err
	}

	// 1. Account.
	accountID, err := b.resolveAccount(ctx, scope, sourceType, req)
	if err != nil {
		return OrderBillingResult{}, err
	}
	res := OrderBillingResult{AccountID: accountID}

	// 2. Contract dibuat sebelum invoice awal supaya invoice dapat menautkan item kontrak periode pertama.
	var contract domain.Contract
	if len(recurring) > 0 {
		contract, err = b.ensureContract(ctx, scope, sourceType, accountID, start, req, recurring)
		if err != nil {
			return OrderBillingResult{}, err
		}
		res.ContractID = contract.ID
	}

	// 3. Invoice awal.
	if len(prepaid) > 0 {
		in := InvoiceInput{
			AccountID: accountID, SourceType: sourceType, SourceID: req.SourceID, IdempotencyKey: "initial",
			Currency: req.Currency, Notes: req.Notes, PICUserID: req.PICUserID, Channels: req.Channels,
		}
		var periodStart, periodEnd *time.Time
		samePeriod := true
		for _, c := range prepaid {
			li := c.line.lineInput(c.attrs)
			if c.recurringIdx >= 0 {
				from, to := pricing.PeriodRange(start, c.attrs.Frequency, 0)
				li.PeriodStart, li.PeriodEnd = &from, &to
				li.Description = fmt.Sprintf("%s (periode %s – %s)", c.line.Description, indoDate(from), indoDate(to))
				if c.recurringIdx < len(contract.Items) {
					li.ContractItemID = contract.Items[c.recurringIdx].ID
				}
				in.ContractID = contract.ID
				if periodStart == nil {
					periodStart, periodEnd = &from, &to
				} else if !periodStart.Equal(from) || !periodEnd.Equal(to) {
					samePeriod = false
				}
			}
			in.Lines = append(in.Lines, li)
		}
		if samePeriod {
			in.PeriodStart, in.PeriodEnd = periodStart, periodEnd
		}
		inv, err := b.invoices.CreateAndIssue(ctx, scope, in, req.ActorUserID)
		if err != nil {
			return OrderBillingResult{}, err
		}
		res.InitialInvoiceID = inv.ID
	}
	return res, nil
}

func (b *orderBilling) BillDelivery(ctx context.Context, scope coretenant.Scope, sourceType domain.SourceType, sourceID, batchKey string,
	lines []OrderLine, req OrderBillingRequest) (string, error) {
	if sourceID == "" || strings.TrimSpace(batchKey) == "" || len(lines) == 0 {
		return "", invalidInvoice("Sumber, kunci batch, dan baris wajib diisi.")
	}
	if sourceType == "" {
		sourceType = domain.SourceSalesOrder
	}
	accountID, err := b.resolveAccount(ctx, scope, sourceType, req)
	if err != nil {
		return "", err
	}
	in := InvoiceInput{
		AccountID: accountID, SourceType: sourceType, SourceID: sourceID, IdempotencyKey: "delivery:" + batchKey,
		Currency: req.Currency, Notes: req.Notes, PICUserID: req.PICUserID, Channels: req.Channels,
	}
	for _, l := range lines {
		attrs, err := l.attrs()
		if err != nil {
			return "", invalidInvoice("Atribut harga baris \"%s\" tidak valid.", l.Description)
		}
		in.Lines = append(in.Lines, l.lineInput(attrs))
	}
	inv, err := b.invoices.CreateAndIssue(ctx, scope, in, req.ActorUserID)
	if err != nil {
		return "", err
	}
	return inv.ID, nil
}

// resolveAccount: account dari sumber (diperbarui bila datanya berubah) atau baru. Tanpa sumber, account
// dipulihkan dari contract/invoice yang sudah lahir dari order ini agar retry tidak menggandakan account.
func (b *orderBilling) resolveAccount(ctx context.Context, scope coretenant.Scope, sourceType domain.SourceType, req OrderBillingRequest) (string, error) {
	p, err := normalizeAccount(req.Account, true)
	if err != nil {
		return "", err
	}
	if p.SourceID != "" {
		found, err := b.accounts.FindBySource(ctx, scope, p.SourceType, p.SourceID)
		switch {
		case err == nil:
			if found.Name != p.Name || found.CompanyName != p.CompanyName || found.Email != p.Email || found.Phone != p.Phone || found.Address != p.Address {
				if found, err = b.accounts.Update(ctx, scope, found.ID, p); err != nil {
					return "", err
				}
			}
			return found.ID, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return "", err
		}
		created, err := b.accounts.Create(ctx, scope, p, req.ActorUserID)
		if errors.Is(err, repository.ErrDuplicateAccountSource) { // balapan: pembuat lain menang
			created, err = b.accounts.FindBySource(ctx, scope, p.SourceType, p.SourceID)
		}
		return created.ID, err
	}
	if c, err := b.contracts.FindBySource(ctx, scope, string(sourceType), req.SourceID); err == nil {
		return c.AccountID, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	existing, _, err := b.invoices.List(ctx, scope, repository.InvoiceListFilter{SourceType: string(sourceType), SourceID: req.SourceID, Limit: 1})
	if err != nil {
		return "", err
	}
	if len(existing) > 0 {
		return existing[0].AccountID, nil
	}
	created, err := b.accounts.Create(ctx, scope, p, req.ActorUserID)
	return created.ID, err
}

func (b *orderBilling) ensureContract(ctx context.Context, scope coretenant.Scope, sourceType domain.SourceType, accountID string,
	start time.Time, req OrderBillingRequest, recurring []classifiedLine) (domain.Contract, error) {
	existing, err := b.contracts.FindBySource(ctx, scope, string(sourceType), req.SourceID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Contract{}, err
	}
	channels, err := normalizeChannels(orChannels(req.Channels))
	if err != nil {
		return domain.Contract{}, err
	}
	items := make([]repository.ContractItemParams, len(recurring))
	for i, c := range recurring {
		index := 0
		if c.attrs.PaymentTiming == pricing.Prepaid {
			index = 1 // periode pertama sudah ditagih di invoice awal
		}
		from, to := pricing.PeriodRange(start, c.attrs.Frequency, index)
		items[i] = repository.ContractItemParams{
			Description: c.line.Description, Quantity: orDefault(c.line.Quantity, "1"), Unit: c.line.Unit,
			UnitPrice: orDefault(c.line.UnitPrice, "0"), DiscountPercent: c.line.DiscountPercent,
			TaxPercent: orDefault(c.line.TaxPercent, "0"), ProductID: c.line.ProductID, SKU: c.line.SKU,
			SourceLineID: c.line.SourceLineID, Frequency: c.attrs.Frequency, PaymentTiming: c.attrs.PaymentTiming,
			PeriodIndex: index, NextPeriodStart: from, NextPeriodEnd: to, Features: c.line.Features,
		}
	}
	number, err := b.nextContractNumber(ctx, scope)
	if err != nil {
		return domain.Contract{}, err
	}
	created, err := b.contracts.Create(ctx, scope, repository.CreateContractParams{
		Number: number, AccountID: accountID, SourceType: string(sourceType), SourceID: req.SourceID,
		Currency: req.Currency, Notes: req.Notes, PICUserID: req.PICUserID, CreatedBy: req.ActorUserID,
		StartDate: start, Channels: channels, Items: items,
	})
	if errors.Is(err, repository.ErrDuplicateSource) { // balapan: pemanggil lain membuat lebih dulu
		return b.contracts.FindBySource(ctx, scope, string(sourceType), req.SourceID)
	}
	if err != nil {
		return domain.Contract{}, err
	}
	b.registry.ContractCreated(ctx, scope, ContractRef{ID: created.ID, Number: created.ContractNumber, SourceID: req.SourceID, SourceType: sourceType})
	return created, nil
}

func orChannels(c []string) []string {
	if len(c) == 0 {
		return []string{"email"}
	}
	return c
}

func (b *orderBilling) nextContractNumber(ctx context.Context, scope coretenant.Scope) (string, error) {
	year := businesstime.DayOf(b.now()).Year()
	seq, err := b.counters.Next(ctx, scope, "contract", year)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CTR-%d-%04d", year, seq), nil
}
