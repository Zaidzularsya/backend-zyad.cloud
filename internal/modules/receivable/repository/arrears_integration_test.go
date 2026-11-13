//go:build integration

package repository_test

import (
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/pricing"
)

func (e env) recording(t *testing.T, scope coretenant.Scope, accountID, contractID, key string) domain.ArrearsRecording {
	t.Helper()
	r, err := e.arrears.CreateRecording(e.ctx, scope, repository.CreateRecordingParams{
		AccountID: accountID, ContractID: contractID, Reason: "Tunggakan lama", IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("create recording: %v", err)
	}
	return r
}

// arrearsInvoice creates and issues one invoice of a recording for period start..end.
func (e env) arrearsInvoice(t *testing.T, scope coretenant.Scope, c domain.Contract, recordingID, start, end, number string, flag bool) domain.Invoice {
	t.Helper()
	p := params(t, c.AccountID, pricing.LineInput{Description: "Tunggakan " + start, UnitPrice: "300000"})
	p.ContractID, p.SourceType, p.SourceID = c.ID, "contract", c.ID
	p.IdempotencyKey = "arrears:" + recordingID + ":" + start
	p.PeriodStart, p.PeriodEnd = date(start), date(end)
	p.ArrearsRecordingID, p.IsolirFlag = recordingID, flag
	inv, err := e.invoices.Create(e.ctx, scope, p)
	if err != nil {
		t.Fatalf("create arrears invoice: %v", err)
	}
	inv, err = e.invoices.Issue(e.ctx, scope, inv.ID, number, *date("2026-11-13"), *date(start), "")
	if err != nil {
		t.Fatalf("issue arrears invoice: %v", err)
	}
	return inv
}

func TestArrears_RecordingKeyIsUniqueAndFindable(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	c := e.contract(t, scope, acc.ID, "CTR-2026-0001", "cccccccc-0000-0000-0000-000000000001")

	first := e.recording(t, scope, acc.ID, c.ID, "key-1")
	if first.ID == "" || first.NotifyStatus != "pending" || first.Reason != "Tunggakan lama" || first.ContractID != c.ID {
		t.Fatalf("recording = %+v", first)
	}
	_, err := e.arrears.CreateRecording(e.ctx, scope, repository.CreateRecordingParams{
		AccountID: acc.ID, ContractID: c.ID, Reason: "lagi", IdempotencyKey: "key-1",
	})
	if !errors.Is(err, repository.ErrDuplicateRecording) {
		t.Fatalf("second create err = %v, want ErrDuplicateRecording", err)
	}
	found, ok, err := e.arrears.FindRecordingByKey(e.ctx, scope, "key-1")
	if err != nil || !ok || found.ID != first.ID {
		t.Fatalf("find by key = %+v ok=%v err=%v", found, ok, err)
	}
	if _, ok, err := e.arrears.FindRecordingByKey(e.ctx, scope, "missing"); err != nil || ok {
		t.Fatalf("missing key ok=%v err=%v", ok, err)
	}
	// Another organization does not see it.
	if _, ok, err := e.arrears.FindRecordingByKey(e.ctx, e.tenants.B.Scope, "key-1"); err != nil || ok {
		t.Fatalf("org B find ok=%v err=%v", ok, err)
	}
}

func TestArrears_ExistingPeriodsIgnoresVoid(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	c := e.contract(t, scope, acc.ID, "CTR-2026-0001", "cccccccc-0000-0000-0000-000000000001")
	rec := e.recording(t, scope, acc.ID, c.ID, "key-1")

	e.arrearsInvoice(t, scope, c, rec.ID, "2026-08-05", "2026-09-04", "INV-2026-0101", false)
	voided := e.arrearsInvoice(t, scope, c, rec.ID, "2026-09-05", "2026-10-04", "INV-2026-0102", false)
	if _, err := e.invoices.Void(e.ctx, scope, voided.ID, "salah catat", ""); err != nil {
		t.Fatal(err)
	}
	// Initial SO invoice: no contract_id, period only on the item that points at the contract item.
	p := params(t, acc.ID, pricing.LineInput{Description: "Internet", UnitPrice: "300000",
		Attrs: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}})
	p.Items[0].ContractItemID = c.Items[0].ID
	p.Items[0].PeriodStart, p.Items[0].PeriodEnd = date("2026-10-05"), date("2026-11-04")
	if _, err := e.invoices.Create(e.ctx, scope, p); err != nil {
		t.Fatal(err)
	}

	got, err := e.arrears.ExistingPeriods(e.ctx, scope, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"2026-08-05": "2026-09-04", "2026-10-05": "2026-11-04"}
	if len(got) != len(want) {
		t.Fatalf("periods = %+v, want %v", got, want)
	}
	for _, r := range got {
		if want[r.Start.Format("2006-01-02")] != r.End.Format("2006-01-02") {
			t.Fatalf("periods = %+v, want %v", got, want)
		}
	}
}

func TestArrears_InvoiceCarriesFlagAndRecording(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	c := e.contract(t, scope, acc.ID, "CTR-2026-0001", "cccccccc-0000-0000-0000-000000000001")
	rec := e.recording(t, scope, acc.ID, c.ID, "key-1")
	inv := e.arrearsInvoice(t, scope, c, rec.ID, "2026-08-05", "2026-09-04", "INV-2026-0101", true)
	if !inv.IsolirFlag || inv.ArrearsRecordingID != rec.ID {
		t.Fatalf("invoice flag=%v recording=%q", inv.IsolirFlag, inv.ArrearsRecordingID)
	}
	list, err := e.arrears.InvoicesByRecording(e.ctx, scope, rec.ID)
	if err != nil || len(list) != 1 || list[0].ID != inv.ID {
		t.Fatalf("invoices by recording = %+v err=%v", list, err)
	}
	normal := e.issued(t, scope, acc.ID, "INV-2026-0001")
	if normal.IsolirFlag || normal.ArrearsRecordingID != "" {
		t.Fatalf("normal invoice flag=%v recording=%q", normal.IsolirFlag, normal.ArrearsRecordingID)
	}
}

func TestArrears_SetIsolirFlagAuditsOnlyChangesOnOpenInvoices(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	c := e.contract(t, scope, acc.ID, "CTR-2026-0001", "cccccccc-0000-0000-0000-000000000001")
	rec := e.recording(t, scope, acc.ID, c.ID, "key-1")
	inv := e.arrearsInvoice(t, scope, c, rec.ID, "2026-08-05", "2026-09-04", "INV-2026-0101", false)

	audits := func() int {
		var n int
		tx, _ := e.db.Begin(e.ctx)
		defer tx.Rollback(e.ctx)
		_, _ = tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID())
		if err := tx.QueryRow(e.ctx, `SELECT count(*) FROM receivable_isolir_flag_audits WHERE organization_id = $1 AND invoice_id = $2`,
			scope.OrganizationID(), inv.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	changed, err := e.arrears.SetIsolirFlag(e.ctx, scope, inv.ID, true, "")
	if err != nil || !changed || audits() != 1 {
		t.Fatalf("first set changed=%v err=%v audits=%d", changed, err, audits())
	}
	changed, err = e.arrears.SetIsolirFlag(e.ctx, scope, inv.ID, true, "")
	if err != nil || changed || audits() != 1 {
		t.Fatalf("same value changed=%v err=%v audits=%d", changed, err, audits())
	}
	got, _ := e.invoices.FindByID(e.ctx, scope, inv.ID)
	if !got.IsolirFlag {
		t.Fatal("flag not stored")
	}
	if _, err := e.invoices.Void(e.ctx, scope, inv.ID, "salah catat", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.arrears.SetIsolirFlag(e.ctx, scope, inv.ID, false, ""); !errors.Is(err, repository.ErrInvoiceNotOpen) {
		t.Fatalf("void invoice err = %v, want ErrInvoiceNotOpen", err)
	}
	if _, err := e.arrears.SetIsolirFlag(e.ctx, e.tenants.B.Scope, inv.ID, true, ""); err == nil {
		t.Fatal("org B must not change org A invoice")
	}
}

func TestArrears_SetNotifyAndGetRecording(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	c := e.contract(t, scope, acc.ID, "CTR-2026-0001", "cccccccc-0000-0000-0000-000000000001")
	rec := e.recording(t, scope, acc.ID, c.ID, "key-1")
	if err := e.arrears.SetNotify(e.ctx, scope, rec.ID, "failed", "tanpa kontak"); err != nil {
		t.Fatal(err)
	}
	got, err := e.arrears.GetRecording(e.ctx, scope, rec.ID)
	if err != nil || got.NotifyStatus != "failed" || got.NotifyError != "tanpa kontak" || got.NotifiedAt == nil {
		t.Fatalf("recording = %+v err=%v", got, err)
	}
	if err := e.arrears.SetNotify(e.ctx, scope, rec.ID, "sent", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = e.arrears.GetRecording(e.ctx, scope, rec.ID)
	if got.NotifyStatus != "sent" || got.NotifyError != "" {
		t.Fatalf("recording = %+v", got)
	}
}

func TestArrears_OpenPeriodIsUniqueButVoidFreesIt(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	c := e.contract(t, scope, acc.ID, "CTR-2026-0001", "cccccccc-0000-0000-0000-000000000001")
	r1 := e.recording(t, scope, acc.ID, c.ID, "key-1")
	r2 := e.recording(t, scope, acc.ID, c.ID, "key-2")
	first := e.arrearsInvoice(t, scope, c, r1.ID, "2026-08-05", "2026-09-04", "INV-2026-0101", false)

	p := params(t, acc.ID, pricing.LineInput{Description: "Tunggakan", UnitPrice: "1"})
	p.ContractID, p.SourceType, p.SourceID, p.IdempotencyKey = c.ID, "contract", c.ID, "arrears:"+r2.ID+":2026-08-05"
	p.PeriodStart, p.PeriodEnd, p.ArrearsRecordingID = date("2026-08-05"), date("2026-09-04"), r2.ID
	if _, err := e.invoices.Create(e.ctx, scope, p); !errors.Is(err, repository.ErrDuplicateArrearsPeriod) {
		t.Fatalf("second open arrears invoice err = %v, want ErrDuplicateArrearsPeriod", err)
	}
	if _, err := e.invoices.Void(e.ctx, scope, first.ID, "salah catat", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.invoices.Create(e.ctx, scope, p); err != nil {
		t.Fatalf("after void: %v", err)
	}
	// A normal invoice of the same contract period is not affected by the arrears index.
	n := params(t, acc.ID, pricing.LineInput{Description: "Internet", UnitPrice: "1"})
	n.ContractID, n.PeriodStart, n.PeriodEnd = c.ID, date("2026-08-05"), date("2026-09-04")
	if _, err := e.invoices.Create(e.ctx, scope, n); err != nil {
		t.Fatalf("normal invoice: %v", err)
	}
}

func TestArrears_ReleaseRecordingKeyFreesClientKey(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	c := e.contract(t, scope, acc.ID, "CTR-2026-0001", "cccccccc-0000-0000-0000-000000000001")
	r := e.recording(t, scope, acc.ID, c.ID, "key-1")
	if err := e.arrears.ReleaseRecordingKey(e.ctx, scope, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := e.arrears.FindRecordingByKey(e.ctx, scope, "key-1"); ok {
		t.Fatal("released key still found")
	}
	e.recording(t, scope, acc.ID, c.ID, "key-1")
}
