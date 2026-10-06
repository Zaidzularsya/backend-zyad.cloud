package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/pricing"
)

const (
	EventSalesOrderCreated = "crm.sales_order_created"

	soStartDateMaxPastDays = 30
	billingFailedMessage   = "Penagihan gagal dibuat. Coba lagi beberapa saat."
)

type salesOrderService struct {
	repo    repository.SalesOrderRepository
	billing receivableservice.OrderBilling
	deps    SalesOrderDeps
	won     DealWonEvaluator
	now     func() time.Time
}

func NewSalesOrderService(repo repository.SalesOrderRepository, billing receivableservice.OrderBilling, deps SalesOrderDeps,
	won DealWonEvaluator, now func() time.Time) SalesOrderService {
	if now == nil {
		now = time.Now
	}
	return &salesOrderService{repo: repo, billing: billing, deps: deps, won: won, now: now}
}

func notFound(err error) error {
	return crmmodule.MapNotFound(err, "SALES_ORDER_NOT_FOUND", "sales order not found")
}

// ---- hook approve ----

func (s *salesOrderService) QuotationApproved(ctx context.Context, scope coretenant.Scope, q domain.Quotation, actorUserID string) error {
	if existing, err := s.repo.FindByQuotation(ctx, scope, q.ID); err == nil {
		_ = existing // idempoten: SO sudah ada, tanpa nomor/aktivitas/notifikasi baru
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	seq, err := s.deps.Counters.NextNumber(ctx, scope, "sales_order")
	if err != nil {
		return err
	}
	number := fmt.Sprintf("SO-%d-%04d", s.now().UTC().Year(), seq)

	defaults := repository.SalesOrderDraftFields{PICUserID: actorUserID}
	if q.DealID != nil && s.deps.Deals != nil {
		if deal, err := s.deps.Deals.FindByID(ctx, scope, *q.DealID); err == nil && deal.OwnerUserID != "" {
			defaults.PICUserID = deal.OwnerUserID
		}
	}
	if q.ContactID != nil && s.deps.Contacts != nil {
		if c, err := s.deps.Contacts.FindByID(ctx, scope, *q.ContactID); err == nil {
			defaults.BillToName = strings.TrimSpace(c.FirstName + " " + c.LastName)
			defaults.BillToEmail, defaults.BillToPhone = c.Email, c.Phone
			switch {
			case c.Email != "":
				defaults.Channels = []string{"email"}
			case c.Phone != "":
				defaults.Channels = []string{"whatsapp"}
			}
		}
	}
	if q.CompanyID != nil && s.deps.Companies != nil {
		if co, err := s.deps.Companies.FindByID(ctx, scope, *q.CompanyID); err == nil {
			defaults.BillToCompany = co.Name
			if defaults.BillToName == "" {
				defaults.BillToName = co.Name
			}
		}
	}

	so, created, err := s.repo.CreateFromQuotation(ctx, scope, q, number, defaults, actorUserID)
	if err != nil {
		return err
	}
	if !created {
		return nil
	}
	s.activity(ctx, scope, so.DealID, actorUserID, fmt.Sprintf("Sales order %s dibuat dari penawaran %s", so.SONumber, q.QuotationNumber))
	s.notifyCreated(ctx, scope, so, q.QuotationNumber)
	return nil
}

func (s *salesOrderService) notifyCreated(ctx context.Context, scope coretenant.Scope, so domain.SalesOrder, quotationNumber string) {
	if s.deps.Notifier == nil || s.deps.Members == nil || so.PICUserID == "" {
		return
	}
	members, err := s.deps.Members.ListActive(ctx, scope)
	if err != nil {
		slog.WarnContext(ctx, "sales order notify: list members failed", "error", err)
		return
	}
	for _, m := range members {
		if m.UserID != so.PICUserID || m.Email == "" {
			continue
		}
		_, err := s.deps.Notifier.Publish(ctx, notificationpublisher.Event{
			Type: EventSalesOrderCreated, OrganizationID: scope.OrganizationID(), UserID: m.UserID,
			Recipient: notificationdomain.NotificationRecipient{Type: "user", UserID: m.UserID, Name: m.Name, Email: m.Email},
			Payload: map[string]any{"app_name": s.deps.AppName, "user_name": m.Name, "so_number": so.SONumber,
				"quotation_number": quotationNumber, "so_url": s.deps.soURL(so.ID)},
			Locale: s.deps.Locale,
		})
		if err != nil {
			slog.WarnContext(ctx, "sales order notify failed", "sales_order_id", so.ID, "error", err)
		}
		return
	}
}

// activity mencatat aktivitas `order` pada deal (best effort; kegagalan hanya di-log).
func (s *salesOrderService) activity(ctx context.Context, scope coretenant.Scope, dealID *string, userID, subject string) {
	if s.deps.Activities == nil || dealID == nil {
		return
	}
	_, err := s.deps.Activities.Create(ctx, scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityDeal, RelatedEntityID: *dealID, Type: domain.ActivityTypeOrder,
		Subject: subject, Status: domain.ActivityStatusCompleted, CreatedBy: userID,
	})
	if err != nil {
		slog.WarnContext(ctx, "sales order activity failed", "deal_id", *dealID, "error", err)
	}
}

func (s *salesOrderService) evaluateWon(ctx context.Context, scope coretenant.Scope, dealID *string) {
	if s.won == nil || dealID == nil {
		return
	}
	if _, err := s.won.Evaluate(ctx, scope, *dealID); err != nil {
		slog.WarnContext(ctx, "won evaluation failed", "deal_id", *dealID, "error", err)
	}
}

// ---- baca ----

func (s *salesOrderService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.SalesOrder, error) {
	so, err := s.repo.FindByID(ctx, scope, id)
	return so, notFound(err)
}

func (s *salesOrderService) List(ctx context.Context, scope coretenant.Scope, f repository.SalesOrderListFilter) ([]domain.SalesOrder, int64, error) {
	return s.repo.List(ctx, scope, f)
}

func (s *salesOrderService) ListByDeal(ctx context.Context, scope coretenant.Scope, dealID string) ([]domain.SalesOrder, error) {
	return s.repo.ListByDeal(ctx, scope, dealID)
}

// ---- draft ----

func (s *salesOrderService) UpdateDraft(ctx context.Context, scope coretenant.Scope, id string, f repository.SalesOrderDraftFields, userID string) (domain.SalesOrder, error) {
	f.BillToName, f.BillToCompany = strings.TrimSpace(f.BillToName), strings.TrimSpace(f.BillToCompany)
	f.BillToEmail, f.BillToPhone, f.BillToAddress = strings.TrimSpace(f.BillToEmail), strings.TrimSpace(f.BillToPhone), strings.TrimSpace(f.BillToAddress)
	if utf8.RuneCountInString(f.BillToName) > 200 || utf8.RuneCountInString(f.BillToCompany) > 200 ||
		utf8.RuneCountInString(f.BillToPhone) > 50 || len(f.BillToEmail) > 255 {
		return domain.SalesOrder{}, fmt.Errorf("%w: panjang field melebihi batas", ErrInvalidSalesOrderFields)
	}
	if f.BillToEmail != "" && !validEmail(f.BillToEmail) {
		return domain.SalesOrder{}, fmt.Errorf("%w: email penagihan tidak valid", ErrInvalidSalesOrderFields)
	}
	seen := map[string]bool{}
	channels := make([]string, 0, len(f.Channels))
	for _, c := range f.Channels {
		if c != "email" && c != "whatsapp" {
			return domain.SalesOrder{}, fmt.Errorf("%w: kanal harus email atau whatsapp", ErrInvalidSalesOrderFields)
		}
		if !seen[c] {
			seen[c] = true
			channels = append(channels, c)
		}
	}
	f.Channels = channels
	if f.PICUserID != "" && s.deps.Members != nil {
		ok, err := s.deps.Members.IsActiveMember(ctx, scope, f.PICUserID)
		if err != nil {
			return domain.SalesOrder{}, err
		}
		if !ok {
			return domain.SalesOrder{}, fmt.Errorf("%w: PIC harus anggota aktif organisasi ini", ErrInvalidSalesOrderFields)
		}
	}
	if f.StartDate != nil {
		d := utcDay(*f.StartDate)
		f.StartDate = &d
	}
	so, err := s.repo.UpdateDraft(ctx, scope, id, f, userID)
	return so, notFound(err)
}

func validEmail(v string) bool {
	addr, err := mail.ParseAddress(v)
	return err == nil && addr.Address == v
}

func utcDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (s *salesOrderService) Cancel(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.SalesOrder, error) {
	so, err := s.repo.Cancel(ctx, scope, id, userID)
	if err != nil {
		return domain.SalesOrder{}, notFound(err)
	}
	s.activity(ctx, scope, so.DealID, userID, fmt.Sprintf("Sales order %s dibatalkan", so.SONumber))
	return so, nil
}

// ---- konfirmasi ----

// validateForConfirm menerapkan syarat konfirmasi (R7).
func (s *salesOrderService) validateForConfirm(so domain.SalesOrder) *IncompleteError {
	var fields []string
	if so.StartDate == nil || utcDay(*so.StartDate).Before(businesstime.DayOf(s.now()).AddDate(0, 0, -soStartDateMaxPastDays)) {
		fields = append(fields, "start_date")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(so.BillToName)); n < 2 || n > 200 {
		fields = append(fields, "bill_to_name")
	}
	if len(so.Channels) == 0 {
		fields = append(fields, "channels")
	}
	for _, c := range so.Channels {
		switch c {
		case "email":
			if !validEmail(so.BillToEmail) {
				fields = append(fields, "bill_to_email")
			}
		case "whatsapp":
			if so.ContactID == nil {
				fields = append(fields, "contact_id")
			}
			if strings.TrimSpace(so.BillToPhone) == "" {
				fields = append(fields, "bill_to_phone")
			}
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return &IncompleteError{Fields: fields}
}

func (s *salesOrderService) Confirm(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.SalesOrder, error) {
	so, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.SalesOrder{}, notFound(err)
	}
	if so.Status != domain.SalesOrderDraft {
		return domain.SalesOrder{}, repository.ErrSalesOrderNotDraft
	}
	if inc := s.validateForConfirm(so); inc != nil {
		return domain.SalesOrder{}, inc
	}
	so, err = s.repo.MarkConfirmed(ctx, scope, id, userID)
	if err != nil {
		return domain.SalesOrder{}, notFound(err)
	}
	return s.bill(ctx, scope, so, userID), nil
}

func (s *salesOrderService) RetryBilling(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.SalesOrder, error) {
	so, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.SalesOrder{}, notFound(err)
	}
	retryable := (so.Status == domain.SalesOrderConfirmed || so.Status == domain.SalesOrderCompleted) &&
		(so.BillingStatus == domain.BillingFailed || so.BillingStatus == domain.BillingPending)
	if !retryable {
		return domain.SalesOrder{}, ErrBillingNotRetryable
	}
	return s.bill(ctx, scope, so, userID), nil
}

func orderLines(items []domain.SalesOrderItem) []receivableservice.OrderLine {
	lines := make([]receivableservice.OrderLine, len(items))
	for i, it := range items {
		lines[i] = receivableservice.OrderLine{
			SourceLineID: it.ID, Description: it.Description, Quantity: it.Quantity, Unit: it.Unit, UnitPrice: it.UnitPrice,
			DiscountPercent: it.DiscountPercent, TaxPercent: it.TaxPercent, ProductID: it.ProductID, SKU: it.SKU,
			Attrs: pricing.Attributes{ChargeType: it.Pricing.ChargeType, Frequency: it.Pricing.Frequency, PaymentTiming: it.Pricing.PaymentTiming},
		}
	}
	return lines
}

func billingRequest(so domain.SalesOrder, actor string, lines []receivableservice.OrderLine) receivableservice.OrderBillingRequest {
	acc := receivableservice.AccountInput{
		Name: so.BillToName, CompanyName: so.BillToCompany, Email: so.BillToEmail, Phone: so.BillToPhone, Address: so.BillToAddress,
	}
	if so.ContactID != nil {
		acc.SourceType, acc.SourceID = receivabledomain.SourceTypeCRMContact, *so.ContactID
	}
	req := receivableservice.OrderBillingRequest{
		SourceType: receivabledomain.SourceSalesOrder, SourceID: so.ID, Account: acc, Channels: so.Channels,
		PICUserID: so.PICUserID, Currency: so.Currency, ActorUserID: actor, Lines: lines,
	}
	if so.StartDate != nil {
		req.StartDate = *so.StartDate
	}
	return req
}

// bill menagih SO yang sudah confirmed. Galat penagihan tidak dikembalikan sebagai error: SO tetap
// confirmed dengan billing_status failed supaya pengguna bisa mencoba lagi tanpa invoice/kontrak dobel.
func (s *salesOrderService) bill(ctx context.Context, scope coretenant.Scope, so domain.SalesOrder, userID string) domain.SalesOrder {
	res, err := s.billing.BillOrder(ctx, scope, billingRequest(so, userID, orderLines(so.Items)))
	if err != nil {
		slog.ErrorContext(ctx, "sales order billing failed", "sales_order_id", so.ID, "error", err)
		failed, setErr := s.repo.SetBillingResult(ctx, scope, so.ID, domain.BillingFailed, billingFailedMessage, "", "", "")
		if setErr != nil {
			slog.ErrorContext(ctx, "sales order: record billing failure failed", "sales_order_id", so.ID, "error", setErr)
			so.BillingStatus, so.BillingError = domain.BillingFailed, billingFailedMessage
			return so
		}
		return failed
	}
	done, err := s.repo.SetBillingResult(ctx, scope, so.ID, domain.BillingDone, "", res.AccountID, res.InitialInvoiceID, res.ContractID)
	if err != nil {
		slog.ErrorContext(ctx, "sales order: record billing result failed", "sales_order_id", so.ID, "error", err)
		return so // tetap pending; Coba lagi aman karena BillOrder idempoten
	}
	s.activity(ctx, scope, done.DealID, userID, s.confirmedSubject(ctx, scope, done))
	s.evaluateWon(ctx, scope, done.DealID)
	return done
}

func (s *salesOrderService) confirmedSubject(ctx context.Context, scope coretenant.Scope, so domain.SalesOrder) string {
	subject := fmt.Sprintf("%s dikonfirmasi", so.SONumber)
	if so.InitialInvoiceID != "" && s.deps.Invoices != nil {
		if _, number, err := s.deps.Invoices.InvoiceStatus(ctx, scope, so.InitialInvoiceID); err == nil && number != "" {
			subject += fmt.Sprintf(". Invoice %s diterbitkan", number)
		}
	}
	if so.ContractID != "" && s.deps.Contracts != nil {
		if number, err := s.deps.Contracts.ContractNumber(ctx, scope, so.ContractID); err == nil && number != "" {
			subject += fmt.Sprintf(". Kontrak %s dibuat", number)
		}
	}
	return subject
}

// ---- konfirmasi diterima ----

func (s *salesOrderService) ConfirmDelivery(ctx context.Context, scope coretenant.Scope, id string, in DeliveryInput, userID string) (domain.SalesOrder, error) {
	in.BatchKey = strings.TrimSpace(in.BatchKey)
	if in.BatchKey == "" {
		return domain.SalesOrder{}, ErrBatchKeyRequired
	}
	if len(in.ItemIDs) == 0 {
		return domain.SalesOrder{}, ErrNothingToDeliver
	}
	today := businesstime.DayOf(s.now())
	at := today
	if !in.DeliveredAt.IsZero() {
		at = utcDay(in.DeliveredAt)
	}
	if at.After(today) {
		return domain.SalesOrder{}, ErrDeliveryDateInFuture
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > 500 {
		return domain.SalesOrder{}, fmt.Errorf("%w: catatan maksimal 500 karakter", ErrInvalidSalesOrderFields)
	}

	so, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.SalesOrder{}, notFound(err)
	}
	if so.Status != domain.SalesOrderConfirmed && so.Status != domain.SalesOrderCompleted {
		return domain.SalesOrder{}, ErrSalesOrderNotConfirmed
	}
	byID := map[string]domain.SalesOrderItem{}
	for _, it := range so.Items {
		byID[it.ID] = it
	}
	var selected []domain.SalesOrderItem
	seen := map[string]bool{}
	for _, itemID := range in.ItemIDs {
		it, ok := byID[itemID]
		if !ok || it.DeliveryStatus != domain.DeliveryPending || seen[itemID] {
			return domain.SalesOrder{}, repository.ErrDeliveryNotPending
		}
		seen[itemID] = true
		selected = append(selected, it)
	}

	invoiceID, err := s.billing.BillDelivery(ctx, scope, receivabledomain.SourceSalesOrder, so.ID, in.BatchKey,
		orderLines(selected), billingRequest(so, userID, nil))
	if err != nil {
		slog.ErrorContext(ctx, "sales order delivery billing failed", "sales_order_id", so.ID, "error", err)
		return domain.SalesOrder{}, fmt.Errorf("%w: %v", ErrBillingFailed, err)
	}
	updated, err := s.repo.MarkDelivered(ctx, scope, so.ID, in.ItemIDs, at, note, invoiceID, userID)
	if err != nil {
		return domain.SalesOrder{}, notFound(err)
	}
	names := make([]string, len(selected))
	for i, it := range selected {
		names[i] = it.Description
	}
	subject := "Barang/layanan diterima: " + strings.Join(names, ", ")
	if s.deps.Invoices != nil {
		if _, number, err := s.deps.Invoices.InvoiceStatus(ctx, scope, invoiceID); err == nil && number != "" {
			subject += "; invoice " + number
		}
	}
	s.activity(ctx, scope, updated.DealID, userID, subject)
	s.evaluateWon(ctx, scope, updated.DealID)
	return updated, nil
}

// ErrBillingFailed membungkus galat penagihan konfirmasi diterima; pesan ramah ditampilkan handler.
var ErrBillingFailed = errors.New("billing failed")
