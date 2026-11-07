package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

type fakePublicDir struct {
	byUser, byPhone map[string][]AccountRef
	userCalls       []string
	phoneCalls      []string
}

func (f *fakePublicDir) AccountsByUsername(_ context.Context, _ coretenant.Scope, u string) ([]AccountRef, error) {
	f.userCalls = append(f.userCalls, u)
	return f.byUser[u], nil
}

func (f *fakePublicDir) AccountsByPhone(_ context.Context, _ coretenant.Scope, p string) ([]AccountRef, error) {
	f.phoneCalls = append(f.phoneCalls, p)
	return f.byPhone[p], nil
}

type fakePublicSecrets struct{ byAcc map[string][]domain.PPPSecret }

func (f fakePublicSecrets) ListByAccount(_ context.Context, _ coretenant.Scope, id string) ([]domain.PPPSecret, error) {
	return f.byAcc[id], nil
}

type fakePublicOverdue struct{ byAcc map[string][]TriggerInvoice }

func (f fakePublicOverdue) TriggerInvoices(_ context.Context, _ coretenant.Scope, id string, _ time.Time) ([]TriggerInvoice, error) {
	return f.byAcc[id], nil
}
func (fakePublicOverdue) AccountsWithTriggerInvoices(context.Context, coretenant.Scope, time.Time, string, int) ([]string, error) {
	return nil, nil
}
func (fakePublicOverdue) AccountOfInvoice(context.Context, coretenant.Scope, string) (string, error) {
	return "", nil
}

type fakePublicLinks struct{ empty bool }

func (f fakePublicLinks) PayLink(_ context.Context, _ coretenant.Scope, id string) (string, error) {
	if f.empty {
		return "", nil
	}
	return "https://pay.test/" + id, nil
}

type fakePublicProfile struct{}

func (fakePublicProfile) Profile(context.Context, coretenant.Scope) (TenantContact, error) {
	return TenantContact{Name: "ISP", Phone: "0811", Email: "a@b.c"}, nil
}

type fakePublicGate struct{ on bool }

func (f fakePublicGate) Enabled(context.Context, coretenant.Scope) (bool, error) { return f.on, nil }

type publicEnv struct {
	dir *fakePublicDir
	svc *PublicService
}

func newPublicEnv(on, noLink bool, accs map[string][]AccountRef, phones map[string][]AccountRef,
	secrets map[string][]domain.PPPSecret, inv map[string][]TriggerInvoice) publicEnv {
	dir := &fakePublicDir{byUser: accs, byPhone: phones}
	return publicEnv{dir: dir, svc: NewPublicService(PublicServiceDeps{
		Directory: dir,
		Secrets:   fakePublicSecrets{secrets},
		Overdue:   fakePublicOverdue{inv},
		PayLinks:  fakePublicLinks{empty: noLink},
		Profile:   fakePublicProfile{},
		Gate:      fakePublicGate{on: on},
	})}
}

var pubToday = time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)

func twoInvoices() map[string][]TriggerInvoice {
	return map[string][]TriggerInvoice{"a1": {
		{ID: "i1", Number: "INV-1", DueDate: pubToday.AddDate(0, 0, -20), Balance: "100000"},
		{ID: "i2", Number: "INV-2", DueDate: pubToday.AddDate(0, 0, -5), Balance: "50000"},
	}}
}

func TestMaskName(t *testing.T) {
	cases := map[string]string{"Budi Santoso": "B*** S******", "": "Pelanggan", "A": "A", "   ": "Pelanggan"}
	for in, want := range cases {
		if got := MaskName(in); got != want {
			t.Errorf("MaskName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestLookup_UsernameWithDebt(t *testing.T) {
	e := newPublicEnv(true, false,
		map[string][]AccountRef{"budi": {{ID: "a1", Name: "Budi Santoso", Phone: "0812", Email: "x@y"}}}, nil,
		map[string][]domain.PPPSecret{"a1": {{Name: "budi", IsolationState: domain.IsolationIsolated}}}, twoInvoices())
	res, err := e.svc.Lookup(context.Background(), coretenant.Scope{}, "  budi ", pubToday)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Found || !res.Isolated || res.MaskedName != "B*** S******" || len(res.Invoices) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Invoices[0].Number != "INV-1" || res.Invoices[0].PayURL != "https://pay.test/i1" || res.Invoices[0].Balance != "100000" {
		t.Fatalf("unexpected invoice: %+v", res.Invoices[0])
	}
	if !reflect.DeepEqual(e.dir.userCalls, []string{"budi"}) {
		t.Fatalf("username passed untrimmed: %v", e.dir.userCalls)
	}
}

func TestLookup_PhoneFormatsEquivalent(t *testing.T) {
	norm := NormalizePhone("0812-3456-789")
	if norm == "" {
		t.Fatal("fixture phone must normalize")
	}
	for _, q := range []string{"0812-3456-789", "+62812-3456-789", "62812 3456 789", "(0812) 3456789"} {
		e := newPublicEnv(true, false, nil, map[string][]AccountRef{norm: {{ID: "a1", Name: "Budi"}}}, nil, twoInvoices())
		res, err := e.svc.Lookup(context.Background(), coretenant.Scope{}, q, pubToday)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(e.dir.phoneCalls, []string{norm}) || len(e.dir.userCalls) != 0 {
			t.Errorf("query %q: phoneCalls=%v userCalls=%v want [%s]", q, e.dir.phoneCalls, e.dir.userCalls, norm)
		}
		if !res.Found {
			t.Errorf("query %q: not found", q)
		}
	}
}

func TestLookup_UniformNotFoundNoDebtAmbiguous(t *testing.T) {
	users := map[string][]AccountRef{
		"nobody": nil,
		"nodebt": {{ID: "a2", Name: "Siti"}},
		"dup":    {{ID: "a1", Name: "A"}, {ID: "a3", Name: "B"}},
	}
	e := newPublicEnv(true, false, users, nil, nil, twoInvoices())
	for _, q := range []string{"nobody", "nodebt", "dup"} {
		res, err := e.svc.Lookup(context.Background(), coretenant.Scope{}, q, pubToday)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(res, PublicLookupResult{}) {
			t.Errorf("%s: not uniform: %+v", q, res)
		}
	}
	// ambiguous phone
	norm := NormalizePhone("081234567890")
	e = newPublicEnv(true, false, nil, map[string][]AccountRef{norm: {{ID: "a1"}, {ID: "a3"}}}, nil, twoInvoices())
	res, _ := e.svc.Lookup(context.Background(), coretenant.Scope{}, "081234567890", pubToday)
	if !reflect.DeepEqual(res, PublicLookupResult{}) {
		t.Errorf("ambiguous phone not uniform: %+v", res)
	}
}

func TestLookup_FeatureDisabledUniform(t *testing.T) {
	e := newPublicEnv(false, false,
		map[string][]AccountRef{"budi": {{ID: "a1", Name: "Budi"}}}, nil, nil, twoInvoices())
	for _, q := range []string{"budi", "081234567890"} {
		res, err := e.svc.Lookup(context.Background(), coretenant.Scope{}, q, pubToday)
		if err != nil || !reflect.DeepEqual(res, PublicLookupResult{}) {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	}
	if len(e.dir.userCalls)+len(e.dir.phoneCalls) != 0 {
		t.Fatalf("directory called while disabled")
	}
}

func TestLookup_NoSender_NoPayURL(t *testing.T) {
	e := newPublicEnv(true, true,
		map[string][]AccountRef{"budi": {{ID: "a1", Name: "Budi"}}}, nil, nil, twoInvoices())
	res, _ := e.svc.Lookup(context.Background(), coretenant.Scope{}, "budi", pubToday)
	if !res.Found || len(res.Invoices) != 2 {
		t.Fatalf("%+v", res)
	}
	for _, it := range res.Invoices {
		if it.PayURL != "" {
			t.Errorf("PayURL=%q", it.PayURL)
		}
	}
}

func TestLookup_NoSecretAccount_NotIsolated(t *testing.T) {
	e := newPublicEnv(true, false,
		map[string][]AccountRef{"budi": {{ID: "a1", Name: "Budi"}}}, nil, nil, twoInvoices())
	res, _ := e.svc.Lookup(context.Background(), coretenant.Scope{}, "budi", pubToday)
	if !res.Found || res.Isolated {
		t.Fatalf("%+v", res)
	}
}

func TestLookup_JunkQueryNeverMatches(t *testing.T) {
	e := newPublicEnv(true, false, nil, nil, nil, twoInvoices())
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'a'
	}
	for _, q := range []string{"12", "   ", "", "abc", "ab\x00c", "ab\ncd", string(long)} {
		res, err := e.svc.Lookup(context.Background(), coretenant.Scope{}, q, pubToday)
		if err != nil || !reflect.DeepEqual(res, PublicLookupResult{}) {
			t.Errorf("%q: res=%+v err=%v", q, res, err)
		}
	}
	if len(e.dir.phoneCalls) != 0 {
		t.Fatalf("phone lookup for junk: %v", e.dir.phoneCalls)
	}
	for _, u := range e.dir.userCalls {
		if u == "" || u == string(long) || u == "ab\x00c" {
			t.Errorf("invalid username reached directory: %q", u)
		}
	}
}

func TestInfo_Disabled(t *testing.T) {
	e := newPublicEnv(false, false, nil, nil, nil, nil)
	info, err := e.svc.Info(context.Background(), coretenant.Scope{})
	if err != nil || info.Enabled || info.Tenant != nil {
		t.Fatalf("%+v %v", info, err)
	}
	e = newPublicEnv(true, false, nil, nil, nil, nil)
	info, _ = e.svc.Info(context.Background(), coretenant.Scope{})
	if !info.Enabled || info.Tenant == nil || info.Tenant.Name != "ISP" {
		t.Fatalf("%+v", info)
	}
}
