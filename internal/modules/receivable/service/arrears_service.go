package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/pricing"
)

var (
	// ErrArrearsConflict: one or more rows hit an already billed period (see ArrearsConflictError).
	ErrArrearsConflict = errors.New("arrears period already billed")
	// ErrArrearsContractMissing: the account has no such contract.
	ErrArrearsContractMissing = errors.New("Pelanggan belum punya kontrak; onboarding dulu.")
	// ErrArrearsInvalid wraps a validation detail (errors.Is(err, ErrArrearsInvalid)).
	ErrArrearsInvalid = errors.New("invalid arrears recording")
)

const (
	maxArrearsRows      = 24
	maxArrearsReason    = 500
	maxIdempotencyKey   = 100
	arrearsCancelReason = "Pencatatan tunggakan dibatalkan."
)

var arrearsAmountPattern = regexp.MustCompile(`^\d{1,16}(\.\d{1,2})?$`)

func invalidArrears(format string, args ...any) error {
	return &validationError{kind: ErrArrearsInvalid, detail: fmt.Sprintf(format, args...)}
}

// ArrearsConflict names one rejected row (1-based) and why.
type ArrearsConflict struct {
	Row         int
	PeriodStart string // YYYY-MM-DD
	Reason      string
}

type ArrearsConflictError struct{ Conflicts []ArrearsConflict }

func (e *ArrearsConflictError) Error() string {
	parts := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		parts[i] = fmt.Sprintf("baris %d (%s): %s", c.Row, c.PeriodStart, c.Reason)
	}
	return "Periode bentrok: " + strings.Join(parts, "; ")
}
func (e *ArrearsConflictError) Is(target error) bool { return target == ErrArrearsConflict }

type ArrearsRowInput struct {
	PeriodStart time.Time // calendar date, must be a cycle start of the contract
	Amount      string
	DueDate     time.Time // calendar date, <= today (WIB)
	IsolirFlag  bool
}

type ArrearsInput struct {
	AccountID, ContractID, Reason, IdempotencyKey string
	Rows                                          []ArrearsRowInput
}

type ArrearsPreviewRow struct {
	Period   ArrearsPeriod
	Conflict string // "" when the period can be recorded
}

type ArrearsResult struct {
	Recording domain.ArrearsRecording
	Invoices  []domain.Invoice
}

type ArrearsService interface {
	// Preview lists the contract's recordable periods with default amounts and conflicts, and
	// whether any contract item's product has an enabled isolation trigger.
	Preview(ctx context.Context, scope coretenant.Scope, contractID string) ([]ArrearsPreviewRow, bool, error)
	// Record validates every row before writing anything, then creates one issued (or overdue)
	// invoice per row. The same idempotency key returns the first recording's result.
	Record(ctx context.Context, scope coretenant.Scope, in ArrearsInput, actorUserID string) (ArrearsResult, error)
	// Resend sends the summary message again and stores the result.
	Resend(ctx context.Context, scope coretenant.Scope, recordingID, actorUserID string) (domain.ArrearsRecording, error)
	// SetIsolirFlag changes the flag of an open arrears invoice (audited).
	SetIsolirFlag(ctx context.Context, scope coretenant.Scope, invoiceID string, value bool, actorUserID string) (domain.Invoice, error)
}

// TriggerProductChecker reports whether any of the products has an enabled Mikrotik
// isolation trigger (adapter in internal/app; receivable does not import mikrotik).
type TriggerProductChecker interface {
	AnyTriggerProduct(ctx context.Context, scope coretenant.Scope, productIDs []string) (bool, error)
}

// ArrearsNotifier sends the one summary message of a recording and reports
// (status, error text) with status sent|failed|skipped. It never fails the recording.
type ArrearsNotifier interface {
	NotifyArrears(ctx context.Context, scope coretenant.Scope, rec domain.ArrearsRecording, invoices []domain.Invoice, actorUserID string) (status, errText string)
}

// historicalCreator is implemented by *invoiceService (unexported path, no clamp, no auto-send).
type historicalCreator interface {
	InvoiceService
	createHistorical(ctx context.Context, scope coretenant.Scope, h historicalInvoice, actorUserID string) (domain.Invoice, error)
}

type ArrearsDeps struct {
	Arrears   repository.ArrearsRepository
	Contracts repository.ContractRepository
	Invoices  InvoiceService // must be the receivable invoice service (NewInvoiceService)
	Overdue   repository.InvoiceRepository
	Triggers  TriggerProductChecker // nil = no trigger products known
	Notifier  ArrearsNotifier       // nil = summary skipped
	Now       func() time.Time
}

type arrearsService struct {
	repo      repository.ArrearsRepository
	contracts repository.ContractRepository
	invoices  historicalCreator
	overdue   repository.InvoiceRepository
	triggers  TriggerProductChecker
	notifier  ArrearsNotifier
	now       func() time.Time
}

func NewArrearsService(d ArrearsDeps) ArrearsService {
	inv, ok := d.Invoices.(historicalCreator)
	if !ok {
		panic("receivable: ArrearsDeps.Invoices must come from NewInvoiceService")
	}
	overdue := d.Overdue
	if overdue == nil {
		overdue = inv.(*invoiceService).invoices
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &arrearsService{repo: d.Arrears, contracts: d.Contracts, invoices: inv, overdue: overdue,
		triggers: d.Triggers, notifier: d.Notifier, now: now}
}

func (s *arrearsService) contractOf(ctx context.Context, scope coretenant.Scope, contractID, accountID string) (domain.Contract, error) {
	if strings.TrimSpace(contractID) == "" {
		return domain.Contract{}, ErrArrearsContractMissing
	}
	c, err := s.contracts.FindByID(ctx, scope, contractID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Contract{}, ErrArrearsContractMissing
	}
	if err != nil {
		return domain.Contract{}, err
	}
	if accountID != "" && c.AccountID != accountID {
		return domain.Contract{}, ErrArrearsContractMissing
	}
	return c, nil
}

func overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return !aStart.After(bEnd) && !bStart.After(aEnd)
}

const conflictBilled = "Periode ini sudah punya invoice."

func (s *arrearsService) Preview(ctx context.Context, scope coretenant.Scope, contractID string) ([]ArrearsPreviewRow, bool, error) {
	c, err := s.contractOf(ctx, scope, contractID, "")
	if err != nil {
		return nil, false, err
	}
	existing, err := s.repo.ExistingPeriods(ctx, scope, c.ID)
	if err != nil {
		return nil, false, err
	}
	periods := ArrearsPeriods(c, businesstime.DayOf(s.now()))
	out := make([]ArrearsPreviewRow, len(periods))
	for i, p := range periods {
		out[i] = ArrearsPreviewRow{Period: p}
		for _, e := range existing {
			if overlaps(p.Start, p.End, e.Start, e.End) {
				out[i].Conflict = conflictBilled
				break
			}
		}
	}
	hasTrigger, err := s.hasTriggerProduct(ctx, scope, c)
	return out, hasTrigger, err
}

func (s *arrearsService) hasTriggerProduct(ctx context.Context, scope coretenant.Scope, c domain.Contract) (bool, error) {
	if s.triggers == nil {
		return false, nil
	}
	ids := []string{}
	for _, it := range c.Items {
		if it.ProductID != "" {
			ids = append(ids, it.ProductID)
		}
	}
	if len(ids) == 0 {
		return false, nil
	}
	return s.triggers.AnyTriggerProduct(ctx, scope, ids)
}

func (s *arrearsService) result(ctx context.Context, scope coretenant.Scope, rec domain.ArrearsRecording) (ArrearsResult, error) {
	invs, err := s.repo.InvoicesByRecording(ctx, scope, rec.ID)
	return ArrearsResult{Recording: rec, Invoices: invs}, err
}

// plannedRow is a validated row with its contract period.
type plannedRow struct {
	in     ArrearsRowInput
	period ArrearsPeriod
}

// validate checks every row without writing; conflicts are collected for all rows at once.
func (s *arrearsService) validate(ctx context.Context, scope coretenant.Scope, c domain.Contract, rows []ArrearsRowInput, today time.Time) ([]plannedRow, error) {
	byStart := map[string]ArrearsPeriod{}
	for _, p := range ArrearsPeriods(c, today) {
		byStart[p.Start.Format(time.DateOnly)] = p
	}
	planned := make([]plannedRow, len(rows))
	for i, r := range rows {
		key := r.PeriodStart.Format(time.DateOnly)
		p, ok := byStart[key]
		switch {
		case r.PeriodStart.Before(c.StartDate):
			return nil, invalidArrears("Baris %d: periode %s sebelum tanggal mulai kontrak.", i+1, key)
		case !ok:
			return nil, invalidArrears("Baris %d: %s tidak bisa dicatat sebagai tunggakan (bukan awal periode tagih yang sudah berjalan, "+
				"setelah kontrak berakhir, atau periode yang akan ditagih otomatis).", i+1, key)
		}
		if r.DueDate.IsZero() || r.DueDate.After(today) {
			return nil, invalidArrears("Baris %d: jatuh tempo harus hari ini atau sebelumnya.", i+1)
		}
		amount := strings.TrimSpace(r.Amount)
		v, okAmount := new(big.Rat).SetString(amount)
		if !arrearsAmountPattern.MatchString(amount) || !okAmount || v.Sign() <= 0 || v.Cmp(maxMoney) > 0 {
			return nil, invalidArrears("Baris %d: jumlah harus lebih dari 0 dengan paling banyak 2 desimal.", i+1)
		}
		r.Amount = amount
		planned[i] = plannedRow{in: r, period: p}
	}

	existing, err := s.repo.ExistingPeriods(ctx, scope, c.ID)
	if err != nil {
		return nil, err
	}
	conflicts := []ArrearsConflict{}
	seen := map[string]int{}
	for i, pr := range planned {
		key := pr.period.Start.Format(time.DateOnly)
		if first, dup := seen[key]; dup {
			conflicts = append(conflicts, ArrearsConflict{Row: i + 1, PeriodStart: key,
				Reason: fmt.Sprintf("Periode sama dengan baris %d.", first)})
			continue
		}
		seen[key] = i + 1
		for _, e := range existing {
			if overlaps(pr.period.Start, pr.period.End, e.Start, e.End) {
				conflicts = append(conflicts, ArrearsConflict{Row: i + 1, PeriodStart: key, Reason: conflictBilled})
				break
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, &ArrearsConflictError{Conflicts: conflicts}
	}
	return planned, nil
}

func (s *arrearsService) Record(ctx context.Context, scope coretenant.Scope, in ArrearsInput, actorUserID string) (ArrearsResult, error) {
	key := strings.TrimSpace(in.IdempotencyKey)
	if key == "" || utf8.RuneCountInString(key) > maxIdempotencyKey {
		return ArrearsResult{}, invalidArrears("Kunci idempotensi wajib, maksimal %d karakter.", maxIdempotencyKey)
	}
	if rec, ok, err := s.repo.FindRecordingByKey(ctx, scope, key); err != nil {
		return ArrearsResult{}, err
	} else if ok {
		return s.result(ctx, scope, rec)
	}
	reason := strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(reason); n < 1 || n > maxArrearsReason {
		return ArrearsResult{}, invalidArrears("Alasan wajib diisi, 1–%d karakter.", maxArrearsReason)
	}
	if len(in.Rows) < 1 || len(in.Rows) > maxArrearsRows {
		return ArrearsResult{}, invalidArrears("Isi 1–%d baris tunggakan.", maxArrearsRows)
	}
	c, err := s.contractOf(ctx, scope, in.ContractID, strings.TrimSpace(in.AccountID))
	if err != nil {
		return ArrearsResult{}, err
	}
	today := businesstime.DayOf(s.now())
	planned, err := s.validate(ctx, scope, c, in.Rows, today)
	if err != nil {
		return ArrearsResult{}, err
	}

	rec, err := s.repo.CreateRecording(ctx, scope, repository.CreateRecordingParams{
		AccountID: c.AccountID, ContractID: c.ID, Reason: reason, RecordedBy: actorUserID, IdempotencyKey: key,
	})
	if errors.Is(err, repository.ErrDuplicateRecording) {
		// A concurrent request with the same key won: return its result.
		existing, ok, findErr := s.repo.FindRecordingByKey(ctx, scope, key)
		if findErr != nil || !ok {
			return ArrearsResult{}, errors.Join(err, findErr)
		}
		return s.result(ctx, scope, existing)
	}
	if err != nil {
		return ArrearsResult{}, err
	}

	created := []domain.Invoice{}
	anyPastDue := false
	for _, pr := range planned {
		inv, err := s.invoices.createHistorical(ctx, scope, s.historical(c, rec, pr, reason), actorUserID)
		if err != nil {
			s.compensate(ctx, scope, rec, created, actorUserID)
			if errors.Is(err, repository.ErrDuplicateArrearsPeriod) {
				return ArrearsResult{}, &ArrearsConflictError{Conflicts: []ArrearsConflict{{
					Row: len(created) + 1, PeriodStart: pr.period.Start.Format(time.DateOnly), Reason: conflictBilled}}}
			}
			return ArrearsResult{}, err
		}
		created = append(created, inv)
		anyPastDue = anyPastDue || pr.in.DueDate.Before(today)
	}
	if anyPastDue {
		// Same transition the hourly overdue job makes (issued AND due_date < today → overdue).
		if _, err := MarkOverdue(ctx, scope, s.overdue, today); err != nil {
			slog.WarnContext(ctx, "receivable: arrears overdue marking failed", "recording_id", rec.ID, "error", err)
		}
	}
	res, err := s.result(ctx, scope, rec)
	if err != nil {
		return ArrearsResult{}, err
	}
	res.Recording = s.notify(ctx, scope, rec, res.Invoices, actorUserID)
	return res, nil
}

func (s *arrearsService) historical(c domain.Contract, rec domain.ArrearsRecording, pr plannedRow, reason string) historicalInvoice {
	start, end, due := pr.period.Start, pr.period.End, pr.in.DueDate
	label := periodLabel(start, end)
	return historicalInvoice{
		Input: InvoiceInput{
			AccountID: c.AccountID, SourceType: domain.SourceContract, SourceID: c.ID, ContractID: c.ID,
			IdempotencyKey: "arrears:" + rec.ID + ":" + start.Format(time.DateOnly),
			Channels:       c.Channels, PeriodStart: &start, PeriodEnd: &end, DueDate: &due,
			Notes: "Tunggakan dicatat: " + reason,
			Lines: []LineInput{{LineInput: pricing.LineInput{Description: "Tunggakan " + label, Quantity: "1", UnitPrice: pr.in.Amount}}},
		},
		RecordingID: rec.ID, IsolirFlag: pr.in.IsolirFlag,
	}
}

// compensate voids the invoices of a failed recording and frees its key so a retry can run.
func (s *arrearsService) compensate(ctx context.Context, scope coretenant.Scope, rec domain.ArrearsRecording, created []domain.Invoice, actorUserID string) {
	for _, inv := range created {
		if _, err := s.invoices.Void(ctx, scope, inv.ID, arrearsCancelReason, actorUserID); err != nil {
			slog.ErrorContext(ctx, "receivable: arrears compensation void failed", "recording_id", rec.ID, "invoice_id", inv.ID, "error", err)
		}
	}
	if err := s.repo.SetNotify(ctx, scope, rec.ID, domain.ArrearsNotifySkipped, arrearsCancelReason); err != nil {
		slog.WarnContext(ctx, "receivable: arrears cancel status failed", "recording_id", rec.ID, "error", err)
	}
	if err := s.repo.ReleaseRecordingKey(ctx, scope, rec.ID); err != nil {
		slog.ErrorContext(ctx, "receivable: arrears key release failed", "recording_id", rec.ID, "error", err)
	}
}

func (s *arrearsService) notify(ctx context.Context, scope coretenant.Scope, rec domain.ArrearsRecording, invs []domain.Invoice, actorUserID string) domain.ArrearsRecording {
	status, errText := domain.ArrearsNotifySkipped, "Pengiriman pesan tidak dikonfigurasi."
	if s.notifier != nil {
		status, errText = s.notifier.NotifyArrears(ctx, scope, rec, invs, actorUserID)
	}
	if err := s.repo.SetNotify(ctx, scope, rec.ID, status, errText); err != nil {
		slog.WarnContext(ctx, "receivable: arrears notify status failed", "recording_id", rec.ID, "error", err)
		rec.NotifyStatus, rec.NotifyError = status, errText
		return rec
	}
	if got, err := s.repo.GetRecording(ctx, scope, rec.ID); err == nil {
		return got
	}
	rec.NotifyStatus, rec.NotifyError = status, errText
	return rec
}

func (s *arrearsService) Resend(ctx context.Context, scope coretenant.Scope, recordingID, actorUserID string) (domain.ArrearsRecording, error) {
	rec, err := s.repo.GetRecording(ctx, scope, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ArrearsRecording{}, receivable.ErrInvoiceNotFound
	}
	if err != nil {
		return domain.ArrearsRecording{}, err
	}
	res, err := s.result(ctx, scope, rec)
	if err != nil {
		return domain.ArrearsRecording{}, err
	}
	open := []domain.Invoice{}
	for _, inv := range res.Invoices {
		if inv.Status == domain.InvoiceIssued || inv.Status == domain.InvoiceOverdue {
			open = append(open, inv)
		}
	}
	if len(open) == 0 {
		return domain.ArrearsRecording{}, invalidArrears("Tidak ada tunggakan terbuka untuk dikirim ulang.")
	}
	return s.notify(ctx, scope, rec, open, actorUserID), nil
}

func (s *arrearsService) SetIsolirFlag(ctx context.Context, scope coretenant.Scope, invoiceID string, value bool, actorUserID string) (domain.Invoice, error) {
	inv, err := s.invoices.Get(ctx, scope, invoiceID)
	if err != nil {
		return domain.Invoice{}, err
	}
	if inv.ArrearsRecordingID == "" {
		return domain.Invoice{}, invalidArrears("Penanda isolir hanya untuk invoice tunggakan.")
	}
	_, err = s.repo.SetIsolirFlag(ctx, scope, invoiceID, value, actorUserID)
	if errors.Is(err, repository.ErrInvoiceNotOpen) {
		return domain.Invoice{}, invalidArrears("Penanda hanya bisa diubah pada invoice yang belum lunas atau void.")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Invoice{}, receivable.ErrInvoiceNotFound
	}
	if err != nil {
		return domain.Invoice{}, err
	}
	return s.invoices.Get(ctx, scope, invoiceID)
}

var indonesianMonths = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func shortDate(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), indonesianMonths[t.Month()-1], t.Year())
}

// periodLabel: "5 Jul 2026 – 4 Agu 2026".
func periodLabel(start, end time.Time) string { return shortDate(start) + " – " + shortDate(end) }
