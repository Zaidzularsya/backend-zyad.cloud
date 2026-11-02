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
	// RouterReady = isolir_ready, RouterProfile = non-empty isolir_profile_name.
	ready := CandidateSecret{Present: true, RouterReady: true, RouterProfile: true}
	isolated := CandidateSecret{Present: true, RouterReady: true, RouterProfile: true, Isolated: true}
	isolatedNeeds := CandidateSecret{Present: true, RouterReady: true, RouterProfile: true, Isolated: true, NeedsOriginalProfile: true}
	needs := CandidateSecret{Present: true, RouterReady: true, RouterProfile: true, NeedsOriginalProfile: true}
	missing := CandidateSecret{RouterReady: true, RouterProfile: true}
	notReady := CandidateSecret{Present: true}
	noProfile := CandidateSecret{Present: true, RouterReady: true}
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
		{"ready router without isolir profile name", false, []CandidateSecret{noProfile}, CandidateRouterNotReady},
		// collisions: the higher priority wins
		{"exempt + no secret", true, nil, CandidateExempt},
		{"exempt + already isolated", true, []CandidateSecret{isolated}, CandidateExempt},
		{"needs flag on the only (isolated) secret", false, []CandidateSecret{isolatedNeeds}, CandidateAlreadyIsolated},
		{"isolated + normal needing profile", false, []CandidateSecret{isolated, needs}, CandidateNeedsProfile},
		{"needs profile + missing", false, []CandidateSecret{{RouterReady: true, RouterProfile: true, NeedsOriginalProfile: true}}, CandidateNeedsProfile},
		{"missing + router not ready", false, []CandidateSecret{{}}, CandidateSecretMissing},
		{"one ready among not ready", false, []CandidateSecret{notReady, ready}, CandidateWillIsolate},
		{"present on unready router, missing on ready router", false, []CandidateSecret{notReady, missing}, CandidateRouterNotReady},
		{"partly isolated, rest isolatable", false, []CandidateSecret{isolated, ready}, CandidateWillIsolate},
		{"needs flag only on the isolated secret, other actionable", false, []CandidateSecret{isolatedNeeds, ready}, CandidateWillIsolate},
		{"needs flag only on the isolated secret, other missing", false, []CandidateSecret{isolatedNeeds, missing}, CandidateSecretMissing},
		// engine-prediction cases (the engine only cuts normal, present,
		// profile-known secrets on a ready router with an isolir profile)
		{"(a) isolated on ready + normal on unready router", false, []CandidateSecret{isolated, notReady}, CandidateRouterNotReady},
		{"(b) isolated + normal not present", false, []CandidateSecret{isolated, missing}, CandidateSecretMissing},
		{"(c) isolated + normal on router without profile name", false, []CandidateSecret{isolated, noProfile}, CandidateRouterNotReady},
		{"(d) actionable + another needing profile", false, []CandidateSecret{ready, needs}, CandidateWillIsolate},
	} {
		if got := CandidateStatusOf(tc.exempt, tc.secrets); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}

// fakeCandidateSource serves rows (sorted by OldestDue, AccountID) from
// memory and honours CandidateFilter.After like the SQL keyset. infinite
// serves full pages forever. failOnCall (1-based) fails that call only;
// beforeCall may mutate rows between pages.
type fakeCandidateSource struct {
	rows       []CandidateRow
	infinite   bool
	served     int
	summary    CandidateSummary
	err        error
	failOnCall int
	beforeCall func(n int, f *fakeCandidateSource)
	calls      []candidateCall
	sumCutoff  time.Time
	sumToday   time.Time
}

type candidateCall struct {
	today, cutoff time.Time
	f             CandidateFilter
	limit, offset int
}

func candidateAfter(r CandidateRow, c *CandidateCursor) bool {
	if !r.OldestDue.Equal(c.OldestDue) {
		return r.OldestDue.After(c.OldestDue)
	}
	return r.AccountID > c.AccountID
}

func (f *fakeCandidateSource) ListCandidates(_ context.Context, _ coretenant.Scope, today, cutoff time.Time, flt CandidateFilter, limit, offset int) ([]CandidateRow, int64, error) {
	f.calls = append(f.calls, candidateCall{today, cutoff, flt, limit, offset})
	n := len(f.calls)
	if f.beforeCall != nil {
		f.beforeCall(n, f)
	}
	if f.err != nil {
		return nil, 0, f.err
	}
	if f.failOnCall == n {
		return nil, 0, errors.New("page read failed")
	}
	if f.infinite {
		out := make([]CandidateRow, limit)
		for i := range out {
			out[i] = CandidateRow{AccountID: fmt.Sprintf("acc-%08d", f.served+i), AccountName: "Budi", Status: CandidateWillIsolate}
		}
		f.served += limit
		return out, 1 << 40, nil
	}
	rows := f.rows
	total := int64(len(f.rows))
	if flt.After != nil {
		total = 0 // keyset pages do not count
		var rest []CandidateRow
		for _, r := range f.rows {
			if candidateAfter(r, flt.After) {
				rest = append(rest, r)
			}
		}
		rows = rest
	}
	if offset >= len(rows) {
		return []CandidateRow{}, total, nil
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end], total, nil
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
		{AccountName: "  =1+1", Phone: "\u3000+62811", RouterName: " @x", Products: "Paket - 10M", Status: CandidateWillIsolate},
	}}
	var buf bytes.Buffer
	var began int64 = -1
	if err := newPreviewForTest(src, 0).ExportCSV(context.Background(), coretenant.Scope{}, CandidateFilter{}, &buf, func(total int64) { began = total }); err != nil {
		t.Fatal(err)
	}
	if began != 4 {
		t.Fatalf("begin total = %d", began)
	}
	recs := readCSV(t, buf.Bytes())
	if len(recs) != 5 || len(recs[0]) != 10 || recs[0][0] != "Akun" {
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
	if r := recs[4]; r[0] != "'  =1+1" || r[1] != "'\u3000+62811" || r[7] != "' @x" || r[8] != "Paket - 10M" {
		t.Errorf("row4 = %q", r)
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
		if c.limit != candidateExportPage || c.offset != 0 || c.f.Status != CandidateWillIsolate {
			t.Fatalf("call %d = %+v", i, c)
		}
		if i == 0 && c.f.After != nil {
			t.Fatalf("first page with cursor: %+v", c.f.After)
		}
		if i > 0 && (c.f.After == nil || c.f.After.AccountID != fmt.Sprintf("acc-%08d", i*candidateExportPage-1)) {
			t.Fatalf("call %d cursor = %+v", i, c.f.After)
		}
	}
}

func TestCSVExport_KeysetSurvivesInsertsBetweenPages(t *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var rows []CandidateRow
	for i := 0; i < 2500; i++ {
		rows = append(rows, CandidateRow{AccountID: fmt.Sprintf("acc-%05d", i), AccountName: fmt.Sprintf("n%05d", i), OldestDue: day.AddDate(0, 0, i/100)})
	}
	src := &fakeCandidateSource{rows: rows, beforeCall: func(n int, f *fakeCandidateSource) {
		if n == 2 { // a new, older candidate appears before the cursor
			f.rows = append([]CandidateRow{{AccountID: "acc-new", AccountName: "new", OldestDue: day.AddDate(0, 0, -1)}}, f.rows...)
		}
	}}
	var buf bytes.Buffer
	if err := newPreviewForTest(src, 0).ExportCSV(context.Background(), coretenant.Scope{}, CandidateFilter{}, &buf, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	recs := readCSV(t, buf.Bytes())[1:]
	seen := map[string]bool{}
	for _, r := range recs {
		if seen[r[0]] {
			t.Fatalf("duplicated row %s", r[0])
		}
		seen[r[0]] = true
	}
	if len(recs) != 2500 || seen["new"] {
		t.Fatalf("rows = %d (new seen %v)", len(recs), seen["new"])
	}
	if len(src.calls) != 3 {
		t.Fatalf("calls = %d", len(src.calls))
	}
}

func TestCSVExport_FailureAfterBeginReturnsError(t *testing.T) {
	var rows []CandidateRow
	for i := 0; i < 1500; i++ {
		rows = append(rows, CandidateRow{AccountID: fmt.Sprintf("acc-%05d", i)})
	}
	src := &fakeCandidateSource{rows: rows, failOnCall: 2}
	began := false
	var buf bytes.Buffer
	err := newPreviewForTest(src, 0).ExportCSV(context.Background(), coretenant.Scope{}, CandidateFilter{}, &buf, func(int64) { began = true })
	if err == nil || !began {
		t.Fatalf("err = %v began = %v", err, began)
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
	if c := src.calls[0]; len(c.f.AccountIDs) != 3 || c.limit != 3 || !c.f.StatusOnly {
		t.Fatalf("call = %+v", c)
	}
}

func TestOnboardingStatuses_PendingReadFailureDegrades(t *testing.T) {
	repo := &fakeOnboardingRepo{statuses: map[string]repository.AccountStatus{"a1": {Secrets: 1}}}
	svc := NewOnboardingService(repo, fakeSender{})
	svc.SetPendingReader(newPreviewForTest(&fakeCandidateSource{err: errors.New("db password=secret down")}, 0))
	got, err := svc.CustomerStatuses(context.Background(), coretenant.Scope{}, []string{"a1", "a2"})
	if err != nil || len(got) != 2 || got[0].PendingIsolation || got[1].PendingIsolation || got[0].IsolationState != CustomerNormal {
		t.Fatalf("degrade: %+v %v", got, err)
	}
}
