package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/receivable/repository"
)

type fakePayoutRepo struct {
	inserted []repository.NewPayoutAccount
	assigned []repository.NewPayoutAccount
	cancels  int
	rejected int
	err      error
}

func (f *fakePayoutRepo) ListByOrganization(context.Context, string) ([]repository.PayoutAccount, error) {
	return []repository.PayoutAccount{{ID: "p", Status: "pending"}, {ID: "v", Status: "verified"}, {ID: "old", Status: "rejected"}}, nil
}
func (f *fakePayoutRepo) ListPending(context.Context, int, int) ([]repository.PayoutAccountWithOrg, int64, error) {
	return nil, 0, nil
}
func (f *fakePayoutRepo) Find(context.Context, string) (repository.PayoutAccount, error) {
	return repository.PayoutAccount{}, nil
}
func (f *fakePayoutRepo) InsertPending(_ context.Context, in repository.NewPayoutAccount, _ repository.PayoutAudit) (repository.PayoutAccount, error) {
	if f.err != nil {
		return repository.PayoutAccount{}, f.err
	}
	f.inserted = append(f.inserted, in)
	return repository.PayoutAccount{ID: "p1", OrganizationID: in.OrganizationID, BankCode: in.BankCode, BankName: in.BankName, AccountNumber: in.AccountNumber, Status: "pending"}, nil
}
func (f *fakePayoutRepo) Cancel(context.Context, string, string, time.Time, repository.PayoutAudit) (repository.PayoutAccount, error) {
	f.cancels++
	return repository.PayoutAccount{ID: "p1", Status: "cancelled"}, nil
}
func (f *fakePayoutRepo) Reject(context.Context, string, string, time.Time, repository.PayoutAudit) (repository.PayoutAccount, error) {
	f.rejected++
	return repository.PayoutAccount{ID: "p1", Status: "rejected"}, nil
}
func (f *fakePayoutRepo) Verify(context.Context, string, string, time.Time, repository.PayoutAudit) (repository.PayoutAccount, error) {
	if f.err != nil {
		return repository.PayoutAccount{}, f.err
	}
	return repository.PayoutAccount{ID: "p1", Status: "verified"}, nil
}
func (f *fakePayoutRepo) AssignVerified(_ context.Context, in repository.NewPayoutAccount, _ string, _ repository.PayoutAudit) (repository.PayoutAccount, error) {
	f.assigned = append(f.assigned, in)
	return repository.PayoutAccount{ID: "a1", OrganizationID: in.OrganizationID, Status: "verified"}, nil
}
func (f *fakePayoutRepo) SummaryForOrganizations(context.Context, []string) (map[string]repository.PayoutSummary, error) {
	return map[string]repository.PayoutSummary{}, nil
}

type fakePayoutNotifier struct{ events []string }

func (n *fakePayoutNotifier) NotifyPayoutAccount(_ context.Context, event string, _ repository.PayoutAccount) {
	n.events = append(n.events, event)
}

func newPayoutSvc() (*fakePayoutRepo, *fakePayoutNotifier, PayoutAccountService) {
	led, _ := newLedger()
	r, n := &fakePayoutRepo{}, &fakePayoutNotifier{}
	return r, n, NewPayoutAccountService(r, led, n)
}

func validInput() PayoutAccountInput {
	return PayoutAccountInput{BankCode: "BCA", AccountNumber: "123-456 789", AccountHolderName: "PT Acme"}
}

func TestSubmitNormalizesAndValidates(t *testing.T) {
	r, _, svc := newPayoutSvc()
	if _, err := svc.Submit(context.Background(), orgA, validInput(), repository.PayoutAudit{ActorUserID: actorID}); err != nil {
		t.Fatal(err)
	}
	got := r.inserted[0]
	if got.AccountNumber != "123456789" || got.BankName == "" || got.BankName != mustBank(t, "BCA").Name || got.ActorUserID != actorID {
		t.Fatalf("inserted = %+v", got)
	}
	bad := map[string]PayoutAccountInput{
		"4 digit":      {BankCode: "BCA", AccountNumber: "1234", AccountHolderName: "PT Acme"},
		"35 digit":     {BankCode: "BCA", AccountNumber: strings.Repeat("1", 35), AccountHolderName: "PT Acme"},
		"huruf":        {BankCode: "BCA", AccountNumber: "12345abc", AccountHolderName: "PT Acme"},
		"bank asing":   {BankCode: "XXX", AccountNumber: "123456789", AccountHolderName: "PT Acme"},
		"holder 2":     {BankCode: "BCA", AccountNumber: "123456789", AccountHolderName: "AB"},
		"holder blank": {BankCode: "BCA", AccountNumber: "123456789", AccountHolderName: "   "},
	}
	for name, in := range bad {
		if _, err := svc.Submit(context.Background(), orgA, in, repository.PayoutAudit{}); !errors.Is(err, ErrInvalidPayoutAccount) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if len(r.inserted) != 1 {
		t.Fatalf("input invalid tetap disimpan: %d", len(r.inserted))
	}
}

func mustBank(t *testing.T, code string) Bank {
	t.Helper()
	b, ok := FindBank(code)
	if !ok {
		t.Fatalf("bank %s tidak ada", code)
	}
	return b
}

func TestSubmitRequiresCustomerOrg(t *testing.T) {
	r, _, svc := newPayoutSvc()
	if _, err := svc.Submit(context.Background(), orgGhost, validInput(), repository.PayoutAudit{}); !errors.Is(err, ErrSettlementBeneficiaryNotFound) {
		t.Fatalf("ghost err = %v", err)
	}
	if _, err := svc.Submit(context.Background(), orgPlat, validInput(), repository.PayoutAudit{}); !errors.Is(err, ErrSettlementBeneficiaryInvalid) {
		t.Fatalf("platform err = %v", err)
	}
	if len(r.inserted) != 0 {
		t.Fatal("insert terjadi untuk org tidak valid")
	}
}

func TestSubmitNotifiesOwner(t *testing.T) {
	_, n, svc := newPayoutSvc()
	if _, err := svc.Submit(context.Background(), orgA, validInput(), repository.PayoutAudit{}); err != nil {
		t.Fatal(err)
	}
	if len(n.events) != 1 || n.events[0] != EventPayoutSubmitted {
		t.Fatalf("events = %v", n.events)
	}
}

func TestSubmitFailureDoesNotNotify(t *testing.T) {
	r, n, svc := newPayoutSvc()
	r.err = repository.ErrPayoutAccountPendingExists
	if _, err := svc.Submit(context.Background(), orgA, validInput(), repository.PayoutAudit{}); !errors.Is(err, repository.ErrPayoutAccountPendingExists) {
		t.Fatalf("err = %v", err)
	}
	if len(n.events) != 0 {
		t.Fatalf("events = %v", n.events)
	}
}

func TestNilNotifierAllowed(t *testing.T) {
	led, _ := newLedger()
	svc := NewPayoutAccountService(&fakePayoutRepo{}, led, nil)
	if _, err := svc.Submit(context.Background(), orgA, validInput(), repository.PayoutAudit{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Verify(context.Background(), "p1", "", repository.PayoutAudit{}); err != nil {
		t.Fatal(err)
	}
}

func TestRejectRequiresNote(t *testing.T) {
	r, n, svc := newPayoutSvc()
	if _, err := svc.Reject(context.Background(), "p1", "  ", repository.PayoutAudit{}); !errors.Is(err, ErrPayoutNoteRequired) {
		t.Fatalf("err = %v", err)
	}
	if r.rejected != 0 || len(n.events) != 0 {
		t.Fatal("reject berjalan tanpa catatan")
	}
	if _, err := svc.Reject(context.Background(), "p1", "nama tidak cocok", repository.PayoutAudit{}); err != nil {
		t.Fatal(err)
	}
	if len(n.events) != 1 || n.events[0] != EventPayoutRejected {
		t.Fatalf("events = %v", n.events)
	}
}

func TestVerifyNotifies(t *testing.T) {
	_, n, svc := newPayoutSvc()
	if _, err := svc.Verify(context.Background(), "p1", "", repository.PayoutAudit{}); err != nil {
		t.Fatal(err)
	}
	if len(n.events) != 1 || n.events[0] != EventPayoutVerified {
		t.Fatalf("events = %v", n.events)
	}
}

func TestAssignRequiresNoteAndCustomerOrg(t *testing.T) {
	r, n, svc := newPayoutSvc()
	ctx := context.Background()
	if _, err := svc.Assign(ctx, orgA, validInput(), " ", repository.PayoutAudit{}); !errors.Is(err, ErrPayoutNoteRequired) {
		t.Fatalf("note err = %v", err)
	}
	if _, err := svc.Assign(ctx, orgPlat, validInput(), "ok", repository.PayoutAudit{}); !errors.Is(err, ErrSettlementBeneficiaryInvalid) {
		t.Fatalf("platform err = %v", err)
	}
	if _, err := svc.Assign(ctx, orgGhost, validInput(), "ok", repository.PayoutAudit{}); !errors.Is(err, ErrSettlementBeneficiaryNotFound) {
		t.Fatalf("ghost err = %v", err)
	}
	if len(r.assigned) != 0 || len(n.events) != 0 {
		t.Fatal("assign berjalan untuk input tidak valid")
	}
	if _, err := svc.Assign(ctx, orgA, validInput(), "verifikasi manual", repository.PayoutAudit{ActorUserID: actorID}); err != nil {
		t.Fatal(err)
	}
	if len(r.assigned) != 1 || r.assigned[0].AccountNumber != "123456789" || len(n.events) != 1 || n.events[0] != EventPayoutAssigned {
		t.Fatalf("assigned=%+v events=%v", r.assigned, n.events)
	}
}

func TestCancelRejectsEmptyOrg(t *testing.T) {
	r, _, svc := newPayoutSvc()
	if _, err := svc.Cancel(context.Background(), " ", "p1", repository.PayoutAudit{}); !errors.Is(err, repository.ErrPayoutAccountNotFound) {
		t.Fatalf("err = %v", err)
	}
	if r.cancels != 0 {
		t.Fatal("repo dipanggil tanpa org")
	}
	if _, err := svc.Cancel(context.Background(), orgA, "p1", repository.PayoutAudit{}); err != nil {
		t.Fatal(err)
	}
}

func TestOverviewPicksVerifiedAndPending(t *testing.T) {
	_, _, svc := newPayoutSvc()
	ov, err := svc.Overview(context.Background(), orgA)
	if err != nil || ov.Verified == nil || ov.Verified.ID != "v" || ov.Pending == nil || ov.Pending.ID != "p" || len(ov.History) != 3 {
		t.Fatalf("ov=%+v err=%v", ov, err)
	}
}

func TestMaskAccountNumber(t *testing.T) {
	if got := MaskAccountNumber("1234567890"); got != "••••7890" {
		t.Fatalf("got %q", got)
	}
}

func TestBanksCatalog(t *testing.T) {
	if len(Banks()) < 10 {
		t.Fatal("daftar bank terlalu sedikit")
	}
	if _, ok := FindBank("nope"); ok {
		t.Fatal("bank tak dikenal ditemukan")
	}
}

func TestHolderNameLengthBoundary(t *testing.T) {
	_, _, svc := newPayoutSvc()
	in := validInput()
	in.AccountHolderName = strings.Repeat("a", 150)
	if _, err := svc.Submit(context.Background(), orgA, in, repository.PayoutAudit{}); err != nil {
		t.Fatalf("150 karakter: %v", err)
	}
	in.AccountHolderName = strings.Repeat("a", 151)
	if _, err := svc.Submit(context.Background(), orgA, in, repository.PayoutAudit{}); !errors.Is(err, ErrInvalidPayoutAccount) {
		t.Fatalf("151 karakter: %v", err)
	}
}
