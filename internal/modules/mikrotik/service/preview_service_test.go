package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

func TestCandidateStatus_PriorityTable(t *testing.T) {
	ready := CandidateSecret{Present: true, RouterReady: true}
	isolated := CandidateSecret{Present: true, RouterReady: true, Isolated: true}
	needs := CandidateSecret{Present: true, RouterReady: true, NeedsOriginalProfile: true}
	missing := CandidateSecret{RouterReady: true}
	notReady := CandidateSecret{Present: true}
	for _, tc := range []struct {
		name    string
		exempt  bool
		secrets []CandidateSecret
		want    string
	}{
		{"will isolate", false, []CandidateSecret{ready}, CandidateWillIsolate},
		{"exempt", true, []CandidateSecret{ready}, CandidateExempt},
		{"no secret", false, nil, CandidateNoSecret},
		{"already isolated", false, []CandidateSecret{isolated, isolated}, CandidateAlreadyIsolated},
		{"needs profile", false, []CandidateSecret{needs}, CandidateNeedsProfile},
		{"secret missing", false, []CandidateSecret{missing}, CandidateSecretMissing},
		{"router not ready", false, []CandidateSecret{notReady}, CandidateRouterNotReady},
		// collisions: the higher priority wins
		{"exempt + no secret", true, nil, CandidateExempt},
		{"exempt + already isolated", true, []CandidateSecret{isolated}, CandidateExempt},
		{"isolated + needs profile on an isolated secret", false, []CandidateSecret{{Present: true, Isolated: true, NeedsOriginalProfile: true}}, CandidateAlreadyIsolated},
		{"partly isolated + needs profile", false, []CandidateSecret{isolated, needs}, CandidateNeedsProfile},
		{"needs profile + missing", false, []CandidateSecret{{NeedsOriginalProfile: true}}, CandidateNeedsProfile},
		{"missing + router not ready", false, []CandidateSecret{{}}, CandidateSecretMissing},
		{"one ready among not ready", false, []CandidateSecret{notReady, ready}, CandidateWillIsolate},
		{"present on unready router, missing on ready router", false, []CandidateSecret{notReady, missing}, CandidateRouterNotReady},
		{"partly isolated, rest isolatable", false, []CandidateSecret{isolated, ready}, CandidateWillIsolate},
		{"needs profile only on the isolated secret", false, []CandidateSecret{{Present: true, RouterReady: true, Isolated: true, NeedsOriginalProfile: true}, missing}, CandidateWillIsolate},
	} {
		if got := CandidateStatusOf(tc.exempt, tc.secrets); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}

// fakeCandidateSource serves rows from memory; total is len(rows) unless
// infinite, in which case every page is full.
type fakeCandidateSource struct {
	rows      []CandidateRow
	infinite  bool
	summary   CandidateSummary
	err       error
	calls     []candidateCall
	sumCutoff time.Time
	sumToday  time.Time
}

type candidateCall struct {
	today, cutoff time.Time
	f             CandidateFilter
	limit, offset int
}

func (f *fakeCandidateSource) ListCandidates(_ context.Context, _ coretenant.Scope, today, cutoff time.Time, flt CandidateFilter, limit, offset int) ([]CandidateRow, int64, error) {
	f.calls = append(f.calls, candidateCall{today, cutoff, flt, limit, offset})
	if f.err != nil {
		return nil, 0, f.err
	}
	if f.infinite {
		out := make([]CandidateRow, limit)
		for i := range out {
			out[i] = CandidateRow{AccountID: fmt.Sprintf("acc-%d", offset+i), AccountName: "Budi", Status: CandidateWillIsolate}
		}
		return out, 1 << 40, nil
	}
	if offset >= len(f.rows) {
		return []CandidateRow{}, int64(len(f.rows)), nil
	}
	end := offset + limit
	if end > len(f.rows) {
		end = len(f.rows)
	}
	return f.rows[offset:end], int64(len(f.rows)), nil
}

func (f *fakeCandidateSource) Summary(_ context.Context, _ coretenant.Scope, today, cutoff time.Time) (CandidateSummary, error) {
	f.sumToday, f.sumCutoff = today, cutoff
	return f.summary, f.err
}

func newPreviewForTest(src *fakeCandidateSource, grace int) *PreviewService {
	ctl := newFakeControlRepo(domain.ModeActive)
	ctl.ctl.GraceDays = grace
	return NewPreviewService(src, ctl, func() time.Time { return ctlNow })
}

func TestPreviewSummary_Counts(t *testing.T) {
	src := &fakeCandidateSource{summary: CandidateSummary{Target: 4, Exempt: 1, Blocked: 3, AlreadyIsolated: 2, PairedAccounts: 10, Safe: 6}}
	got, err := newPreviewForTest(src, 3).Summary(context.Background(), coretenant.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	today := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	if !src.sumToday.Equal(today) || !src.sumCutoff.Equal(cutoff) {
		t.Fatalf("today/cutoff passed = %s / %s", src.sumToday, src.sumCutoff)
	}
	if got.GraceDays != 3 || !got.Cutoff.Equal(cutoff) || got.Target != 4 || got.Safe != 6 || got.Blocked != 3 ||
		got.Exempt != 1 || got.AlreadyIsolated != 2 || got.PairedAccounts != 10 {
		t.Fatalf("summary = %+v", got)
	}

	// Safe is derived (PairedAccounts - Target) and never negative.
	src.summary = CandidateSummary{Target: 5, PairedAccounts: 7, Safe: 99}
	if got, _ := newPreviewForTest(src, 0).Summary(context.Background(), coretenant.Scope{}); got.Safe != 2 {
		t.Fatalf("safe = %d", got.Safe)
	}
	src.summary = CandidateSummary{Target: 5, PairedAccounts: 3}
	if got, _ := newPreviewForTest(src, 0).Summary(context.Background(), coretenant.Scope{}); got.Safe != 0 {
		t.Fatalf("safe clamp = %d", got.Safe)
	}
}

func TestPreviewList_ClampsAndValidates(t *testing.T) {
	src := &fakeCandidateSource{}
	svc := newPreviewForTest(src, 0)
	ctx := context.Background()
	if _, _, err := svc.List(ctx, coretenant.Scope{}, CandidateFilter{Status: "bogus"}, 1, 10); err == nil {
		t.Fatal("unknown status accepted")
	}
	if _, _, err := svc.List(ctx, coretenant.Scope{}, CandidateFilter{Q: string(bytes.Repeat([]byte("a"), 101))}, 1, 10); err == nil {
		t.Fatal("long q accepted")
	}
	for _, tc := range []struct{ page, per, limit, offset int }{
		{0, 0, 50, 0}, {2, 500, 100, 100}, {3, 10, 10, 20},
	} {
		src.calls = nil
		if _, _, err := svc.List(ctx, coretenant.Scope{}, CandidateFilter{}, tc.page, tc.per); err != nil {
			t.Fatal(err)
		}
		c := src.calls[0]
		if c.limit != tc.limit || c.offset != tc.offset {
			t.Errorf("page %d per %d: limit %d offset %d", tc.page, tc.per, c.limit, c.offset)
		}
		if !c.today.Equal(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)) || !c.cutoff.Equal(c.today) {
			t.Errorf("today/cutoff = %s/%s", c.today, c.cutoff)
		}
	}
}

func readCSV(t *testing.T, b []byte) [][]string {
	t.Helper()
	recs, err := csv.NewReader(bytes.NewReader(b)).ReadAll()
	if err != nil {
		t.Fatalf("csv: %v\n%s", err, b)
	}
	return recs
}

func TestCSVExport_EscapesFormulaCells(t *testing.T) {
	until := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	src := &fakeCandidateSource{rows: []CandidateRow{
		{AccountName: "=HYPERLINK(\"x\")", Phone: "+6281234", Status: CandidateExempt, RouterName: "@rtr", Products: "-promo",
			OverdueDays: 5, InvoiceCount: 2, Balance: "150000.00", ExemptUntil: &until},
		{AccountName: "\tTab", Phone: "\rcr", Status: CandidateWillIsolate, RouterName: "R1", Products: "Paket 10M", OverdueDays: 1, InvoiceCount: 1, Balance: "1.00"},
		{AccountName: "Budi, \"Jr\"", Phone: "0812", Status: CandidateRouterNotReady},
	}}
	var buf bytes.Buffer
	var began int64 = -1
	if err := newPreviewForTest(src, 0).ExportCSV(context.Background(), coretenant.Scope{}, CandidateFilter{}, &buf, func(total int64) { began = total }); err != nil {
		t.Fatal(err)
	}
	if began != 3 {
		t.Fatalf("begin total = %d", began)
	}
	recs := readCSV(t, buf.Bytes())
	if len(recs) != 4 || len(recs[0]) != 10 || recs[0][0] != "Akun" {
		t.Fatalf("records = %q", recs)
	}
	r1 := recs[1]
	want1 := []string{"'=HYPERLINK(\"x\")", "'+6281234", "Dikecualikan", "Tunggakan 5 hari", "5", "2", "150000.00", "'@rtr", "'-promo", "2026-12-31"}
	for i := range want1 {
		if r1[i] != want1[i] {
			t.Errorf("row1 col %d = %q want %q", i, r1[i], want1[i])
		}
	}
	if recs[2][0] != "'\tTab" || recs[2][1] != "'\rcr" || recs[2][2] != "Akan diisolir" {
		t.Errorf("row2 = %q", recs[2])
	}
	if recs[3][0] != "Budi, \"Jr\"" || recs[3][9] != "" {
		t.Errorf("row3 = %q", recs[3])
	}
}

func TestCSVExport_RowCap(t *testing.T) {
	src := &fakeCandidateSource{infinite: true}
	var buf bytes.Buffer
	if err := newPreviewForTest(src, 0).ExportCSV(context.Background(), coretenant.Scope{}, CandidateFilter{Status: CandidateWillIsolate}, &buf, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	recs := readCSV(t, buf.Bytes())
	if len(recs) != MaxCandidateExportRows+1 {
		t.Fatalf("rows = %d", len(recs)-1)
	}
	if len(src.calls) != MaxCandidateExportRows/candidateExportPage {
		t.Fatalf("pages = %d", len(src.calls))
	}
	for i, c := range src.calls {
		if c.limit != candidateExportPage || c.offset != i*candidateExportPage || c.f.Status != CandidateWillIsolate {
			t.Fatalf("call %d = %+v", i, c)
		}
	}
}

func TestCSVExport_ErrorBeforeBeginWritesNothing(t *testing.T) {
	src := &fakeCandidateSource{err: errors.New("db down")}
	var buf bytes.Buffer
	began := false
	err := newPreviewForTest(src, 0).ExportCSV(context.Background(), coretenant.Scope{}, CandidateFilter{}, &buf, func(int64) { began = true })
	if err == nil || began || buf.Len() != 0 {
		t.Fatalf("err = %v began = %v written = %d", err, began, buf.Len())
	}
}

func TestOnboardingStatuses_PendingIsolation(t *testing.T) {
	repo := &fakeOnboardingRepo{statuses: map[string]repository.AccountStatus{}}
	src := &fakeCandidateSource{rows: []CandidateRow{
		{AccountID: "a1", Status: CandidateWillIsolate},
		{AccountID: "a2", Status: CandidateRouterNotReady},
	}}
	svc := NewOnboardingService(repo, fakeSender{})
	got, err := svc.CustomerStatuses(context.Background(), coretenant.Scope{}, []string{"a1", "a2", "a3"})
	if err != nil || got[0].PendingIsolation {
		t.Fatalf("without pending reader: %+v %v", got, err)
	}
	svc.SetPendingReader(newPreviewForTest(src, 0))
	got, err = svc.CustomerStatuses(context.Background(), coretenant.Scope{}, []string{"a1", "a2", "a3"})
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].PendingIsolation || got[1].PendingIsolation || got[2].PendingIsolation {
		t.Fatalf("pending = %+v", got)
	}
	if c := src.calls[0]; len(c.f.AccountIDs) != 3 || c.limit != 3 {
		t.Fatalf("call = %+v", c)
	}
}
