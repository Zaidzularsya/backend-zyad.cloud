package service

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

const (
	// DefaultCandidatePageSize / MaxCandidatePageSize bound the preview list.
	DefaultCandidatePageSize = 50
	MaxCandidatePageSize     = 100
	// MaxCandidateExportRows caps one CSV export.
	MaxCandidateExportRows = 100000
	// candidateExportPage is the ListCandidates page size of the export.
	candidateExportPage = 1000
	// maxCandidateQueryRunes bounds the search text.
	maxCandidateQueryRunes = 100
	// maxCandidateAccountIDs bounds CandidateFilter.AccountIDs.
	maxCandidateAccountIDs = 100
)

// CandidateSecret is the per-secret input of CandidateStatusOf: one paired
// secret of the account on a non-deleted router. RouterReady is the router's
// isolir_ready; RouterProfile tells that its isolir_profile_name is set.
type CandidateSecret struct {
	Present, Isolated, NeedsOriginalProfile, RouterReady, RouterProfile bool
}

// actionable mirrors what the engine really cuts: the evaluator only isolates
// normal, present secrets with a known original profile, and the task only
// succeeds on an isolir-ready router with an isolir profile name.
func (s CandidateSecret) actionable() bool {
	return !s.Isolated && s.Present && !s.NeedsOriginalProfile && s.RouterReady && s.RouterProfile
}

// CandidateStatusOf predicts what the engine does with an account that has
// candidate invoices. exempt is the effective exemption (EffectiveExempt).
// Priority (first match wins): exempt, no_secret, already_isolated (every
// secret isolated), will_isolate (any actionable secret), needs_profile (a
// normal secret waits for its original profile), secret_missing (no normal
// secret is present), router_not_ready. The SQL read model mirrors this
// function; an integration test runs the same table through both.
func CandidateStatusOf(exempt bool, secrets []CandidateSecret) string {
	if exempt {
		return CandidateExempt
	}
	if len(secrets) == 0 {
		return CandidateNoSecret
	}
	allIsolated, anyActionable, needsProfile, anyNormalPresent := true, false, false, false
	for _, s := range secrets {
		allIsolated = allIsolated && s.Isolated
		anyActionable = anyActionable || s.actionable()
		needsProfile = needsProfile || (!s.Isolated && s.NeedsOriginalProfile)
		anyNormalPresent = anyNormalPresent || (!s.Isolated && s.Present)
	}
	switch {
	case allIsolated:
		return CandidateAlreadyIsolated
	case anyActionable:
		return CandidateWillIsolate
	case needsProfile:
		return CandidateNeedsProfile
	case !anyNormalPresent:
		return CandidateSecretMissing
	}
	return CandidateRouterNotReady
}

// ValidCandidateStatus reports whether s is one of the Candidate* statuses.
func ValidCandidateStatus(s string) bool {
	switch s {
	case CandidateExempt, CandidateNoSecret, CandidateAlreadyIsolated, CandidateNeedsProfile,
		CandidateSecretMissing, CandidateRouterNotReady, CandidateWillIsolate:
		return true
	}
	return false
}

// GraceReader reads the tenant control settings; satisfied by
// repository.ControlRepository.
type GraceReader interface {
	Get(ctx context.Context, scope coretenant.Scope) (domain.ControlSettings, error)
}

// PreviewService shows who the engine would isolate today: it resolves the
// business day and the grace cutoff and reads the candidate model.
type PreviewService struct {
	src     CandidateSource
	control GraceReader
	now     func() time.Time
}

// NewPreviewService builds the service; now defaults to time.Now.
func NewPreviewService(src CandidateSource, control GraceReader, now func() time.Time) *PreviewService {
	if now == nil {
		now = time.Now
	}
	return &PreviewService{src: src, control: control, now: now}
}

// window returns today (WIB business day), the candidate cutoff and the grace days.
func (s *PreviewService) window(ctx context.Context, scope coretenant.Scope) (today, cutoff time.Time, grace int, err error) {
	ctl, err := s.control.Get(ctx, scope)
	if err != nil {
		return time.Time{}, time.Time{}, 0, err
	}
	today = businesstime.DayOf(s.now())
	return today, CandidateCutoff(today, ctl.GraceDays), ctl.GraceDays, nil
}

func (s *PreviewService) Summary(ctx context.Context, scope coretenant.Scope) (CandidateSummary, error) {
	today, cutoff, grace, err := s.window(ctx, scope)
	if err != nil {
		return CandidateSummary{}, err
	}
	sum, err := s.src.Summary(ctx, scope, today, cutoff)
	if err != nil {
		return CandidateSummary{}, err
	}
	sum.GraceDays, sum.Cutoff = grace, cutoff
	sum.Safe = sum.PairedAccounts - sum.Target
	if sum.Safe < 0 {
		// A target account always has a paired secret, so this only guards
		// against the two counts being read at slightly different moments.
		sum.Safe = 0
	}
	return sum, nil
}

func validateCandidateFilter(f CandidateFilter) error {
	if f.Status != "" && !ValidCandidateStatus(f.Status) {
		return invalid("unknown candidate status")
	}
	if utf8.RuneCountInString(f.Q) > maxCandidateQueryRunes {
		return invalid("q must be at most 100 characters")
	}
	if len(f.AccountIDs) > maxCandidateAccountIDs {
		return invalid("at most 100 account ids")
	}
	return nil
}

// List returns one page (page from 1; perPage clamped to 1..100, default 50).
func (s *PreviewService) List(ctx context.Context, scope coretenant.Scope, f CandidateFilter, page, perPage int) ([]CandidateRow, int64, error) {
	if err := validateCandidateFilter(f); err != nil {
		return nil, 0, err
	}
	page, perPage = ClampCandidatePage(page, perPage)
	today, cutoff, _, err := s.window(ctx, scope)
	if err != nil {
		return nil, 0, err
	}
	return s.src.ListCandidates(ctx, scope, today, cutoff, f, perPage, (page-1)*perPage)
}

// ClampCandidatePage applies the list defaults: page >= 1, perPage 1..100
// (0 or less = 50).
func ClampCandidatePage(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = DefaultCandidatePageSize
	}
	if perPage > MaxCandidatePageSize {
		perPage = MaxCandidatePageSize
	}
	return page, perPage
}

// PendingIsolation reports, per requested account, whether the engine would
// isolate it today (status will_isolate). Accounts without candidate
// invoices are false. At most 100 ids.
func (s *PreviewService) PendingIsolation(ctx context.Context, scope coretenant.Scope, accountIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	f := CandidateFilter{AccountIDs: accountIDs, StatusOnly: true}
	if err := validateCandidateFilter(f); err != nil {
		return nil, err
	}
	today, cutoff, _, err := s.window(ctx, scope)
	if err != nil {
		return nil, err
	}
	rows, _, err := s.src.ListCandidates(ctx, scope, today, cutoff, f, len(accountIDs), 0)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.AccountID] = r.Status == CandidateWillIsolate
	}
	return out, nil
}

var candidateStatusLabels = map[string]string{
	CandidateExempt:          "Dikecualikan",
	CandidateNoSecret:        "Belum ada PPP secret",
	CandidateAlreadyIsolated: "Sudah terisolir",
	CandidateNeedsProfile:    "Profil asli belum diisi",
	CandidateSecretMissing:   "Secret tidak ditemukan di router",
	CandidateRouterNotReady:  "Router belum siap isolir",
	CandidateWillIsolate:     "Akan diisolir",
}

var candidateCSVHeader = []string{
	"Akun", "Telepon", "Status", "Alasan", "Hari Terlambat", "Jumlah Invoice", "Saldo", "Router", "Produk", "Dikecualikan Sampai",
}

// csvCell neutralises spreadsheet formulas (CSV injection): a cell starting
// with tab or CR, or whose first non-space character (Unicode spaces such as
// U+3000 included) is = + - or @, gets a leading single quote.
func csvCell(v string) string {
	if v == "" {
		return v
	}
	if v[0] == '\t' || v[0] == '\r' {
		return "'" + v
	}
	if t := strings.TrimLeftFunc(v, unicode.IsSpace); t != "" && strings.ContainsRune("=+-@", rune(t[0])) {
		return "'" + v
	}
	return v
}

func candidateCSVRecord(r CandidateRow) []string {
	label := candidateStatusLabels[r.Status]
	if label == "" {
		label = r.Status
	}
	until := ""
	if r.ExemptUntil != nil {
		until = r.ExemptUntil.Format("2006-01-02")
	}
	rec := []string{
		r.AccountName, r.Phone, label, fmt.Sprintf("Tunggakan %d hari", r.OverdueDays),
		strconv.Itoa(r.OverdueDays), strconv.Itoa(r.InvoiceCount), r.Balance, r.RouterName, r.Products, until,
	}
	for i := range rec {
		rec[i] = csvCell(rec[i])
	}
	return rec
}

// ExportCSV writes the filtered candidates as CSV, in keyset pages of 1000,
// up to MaxCandidateExportRows rows. Everything that can fail cheaply
// (filter, settings, the first page with its total) happens before begin is
// called with the total matching rows, so the caller can still answer with a
// JSON error. An error returned after begin means the file is incomplete; the
// caller must not finish the response as if it were whole. Each page is
// flushed (w too when it has Flush()).
func (s *PreviewService) ExportCSV(ctx context.Context, scope coretenant.Scope, f CandidateFilter, w io.Writer, begin func(total int64)) error {
	if err := validateCandidateFilter(f); err != nil {
		return err
	}
	today, cutoff, _, err := s.window(ctx, scope)
	if err != nil {
		return err
	}
	f.After = nil
	rows, total, err := s.src.ListCandidates(ctx, scope, today, cutoff, f, candidateExportPage, 0)
	if err != nil {
		return err
	}
	begin(total)
	cw := csv.NewWriter(w)
	if err := cw.Write(candidateCSVHeader); err != nil {
		return err
	}
	flusher, _ := w.(interface{ Flush() })
	written := 0
	for {
		for _, r := range rows {
			if written >= MaxCandidateExportRows {
				break
			}
			if err := cw.Write(candidateCSVRecord(r)); err != nil {
				return err
			}
			written++
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		if len(rows) < candidateExportPage || written >= MaxCandidateExportRows {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		last := rows[len(rows)-1]
		f.After = &CandidateCursor{OldestDue: last.OldestDue, AccountID: last.AccountID}
		if rows, _, err = s.src.ListCandidates(ctx, scope, today, cutoff, f, candidateExportPage, 0); err != nil {
			return err
		}
	}
}
