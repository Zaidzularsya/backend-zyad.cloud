package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

var ctx = context.Background()

// ---- fakes ----

type fakeOnboardSecrets struct {
	mu      sync.Mutex
	secrets map[string]domain.PPPSecret
	pairs   int
}

func (f *fakeOnboardSecrets) Get(_ context.Context, _ coretenant.Scope, id string) (domain.PPPSecret, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.secrets[id]
	if !ok {
		return domain.PPPSecret{}, mikrotik.ErrSecretNotFound
	}
	return s, nil
}

func (f *fakeOnboardSecrets) CheckOnboardable(_ context.Context, _ coretenant.Scope, s domain.PPPSecret) error {
	if !s.Present {
		return mikrotik.ErrSecretNotPairable
	}
	return nil
}

func (f *fakeOnboardSecrets) Pair(_ context.Context, _ coretenant.Scope, secretID, accountID, _ string) (domain.PPPSecret, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.secrets[secretID]
	if s.AccountID != nil {
		return domain.PPPSecret{}, mikrotik.ErrSecretAlreadyPaired
	}
	f.pairs++
	s.AccountID = &accountID
	f.secrets[secretID] = s
	return s, nil
}

type fakeOnboardCRM struct {
	mu        sync.Mutex
	calls     []string
	contacts  map[string]OnboardContactRef
	accountOf map[string]string // account id -> contact id
	active    map[string]bool   // account id -> has active contract
	failOnce  map[string]error  // step name -> error returned once
	createErr error             // CreateContact error until cleared
	deal      OnboardDeal
	quotation OnboardQuotation
	confirm   ConfirmOrder
	seq       int
	slow      time.Duration
}

func newFakeCRM() *fakeOnboardCRM {
	return &fakeOnboardCRM{
		contacts:  map[string]OnboardContactRef{"c-old": {ID: "c-old", Name: "Budi", Email: "budi@b.com", Phone: "0812"}},
		accountOf: map[string]string{},
		active:    map[string]bool{},
		failOnce:  map[string]error{},
	}
}

func (f *fakeOnboardCRM) record(step string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, step)
	if err, ok := f.failOnce[step]; ok {
		delete(f.failOnce, step)
		return err
	}
	return nil
}

func (f *fakeOnboardCRM) count(step string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == step {
			n++
		}
	}
	return n
}

func (f *fakeOnboardCRM) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for _, c := range f.calls {
		if c != "find_duplicate" && c != "get_contact" && c != "contact_of_account" && c != "has_active_contract" {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeOnboardCRM) FindDuplicateContact(_ context.Context, _ coretenant.Scope, email, phone string) (OnboardContactRef, bool, error) {
	_ = f.record("find_duplicate")
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.contacts {
		if (email != "" && strings.EqualFold(c.Email, strings.TrimSpace(email))) || (phone != "" && c.Phone == phone) {
			return c, true, nil
		}
	}
	return OnboardContactRef{}, false, nil
}

func (f *fakeOnboardCRM) GetContact(_ context.Context, _ coretenant.Scope, id string) (OnboardContactRef, error) {
	_ = f.record("get_contact")
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.contacts[id]
	if !ok {
		return OnboardContactRef{}, mikrotik.ErrContactNotFound
	}
	return c, nil
}

func (f *fakeOnboardCRM) CreateContact(_ context.Context, _ coretenant.Scope, c NewContact, _ string) (OnboardContactRef, error) {
	if f.slow > 0 {
		time.Sleep(f.slow)
	}
	if err := f.record("create_contact"); err != nil {
		return OnboardContactRef{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return OnboardContactRef{}, f.createErr
	}
	f.seq++
	ref := OnboardContactRef{ID: "c-new", Name: c.Name, Email: c.Email, Phone: c.Phone}
	f.contacts[ref.ID] = ref
	return ref, nil
}

func (f *fakeOnboardCRM) ContactOfAccount(_ context.Context, _ coretenant.Scope, accountID string) (string, error) {
	_ = f.record("contact_of_account")
	return f.accountOf[accountID], nil
}

func (f *fakeOnboardCRM) AccountHasActiveContract(_ context.Context, _ coretenant.Scope, accountID string) (bool, error) {
	_ = f.record("has_active_contract")
	return f.active[accountID], nil
}

func (f *fakeOnboardCRM) ContractRef(_ context.Context, _ coretenant.Scope, id string) (ContractRef, error) {
	return ContractRef{ID: id, Number: "CTR-" + id}, nil
}

func (f *fakeOnboardCRM) ActiveContractOfAccount(_ context.Context, _ coretenant.Scope, accountID string) (ContractRef, bool, error) {
	if !f.active[accountID] {
		return ContractRef{}, false, nil
	}
	return ContractRef{ID: "ctr-of-" + accountID, Number: "CTR-L"}, true, nil
}

func (f *fakeOnboardCRM) CreateDeal(_ context.Context, _ coretenant.Scope, d OnboardDeal, _ string) (string, error) {
	if err := f.record("create_deal"); err != nil {
		return "", err
	}
	f.deal = d
	return "deal-" + d.ContactID, nil
}

func (f *fakeOnboardCRM) CreateQuotation(_ context.Context, _ coretenant.Scope, q OnboardQuotation, _ string) (string, error) {
	if err := f.record("create_quotation"); err != nil {
		return "", err
	}
	f.quotation = q
	return "quo-" + q.DealID, nil
}

func (f *fakeOnboardCRM) ApproveQuotation(_ context.Context, _ coretenant.Scope, id, _ string) error {
	return f.record("approve_quotation")
}

func (f *fakeOnboardCRM) EnsureSalesOrder(_ context.Context, _ coretenant.Scope, quotationID, _ string) (string, error) {
	if err := f.record("ensure_sales_order"); err != nil {
		return "", err
	}
	return "so-" + quotationID, nil
}

func (f *fakeOnboardCRM) ConfirmSalesOrder(_ context.Context, _ coretenant.Scope, soID string, o ConfirmOrder, _ string) (ConfirmedOrder, error) {
	if err := f.record("confirm_sales_order"); err != nil {
		return ConfirmedOrder{}, err
	}
	f.confirm = o
	return ConfirmedOrder{AccountID: "acct-" + soID, ContractID: "ctr-" + soID}, nil
}

type fakeOnboardPerms struct{ missing []string }

func (f fakeOnboardPerms) Missing(context.Context, coretenant.Scope, string, bool) ([]string, error) {
	return f.missing, nil
}

type memOnboardStore struct {
	mu    sync.Mutex
	state map[string]OnboardState
}

func (m *memOnboardStore) Get(_ context.Context, _ coretenant.Scope, secretID string) (OnboardState, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.state[secretID]
	return s, ok, nil
}

func (m *memOnboardStore) Save(_ context.Context, _ coretenant.Scope, s OnboardState, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state[s.SecretID] = s
	return nil
}

type mutexLocker struct {
	mu   sync.Mutex
	held map[string]bool
}

func (l *mutexLocker) TryLock(_ context.Context, key string) (func(), bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held[key] {
		return nil, false, nil
	}
	l.held[key] = true
	return func() { l.mu.Lock(); delete(l.held, key); l.mu.Unlock() }, true, nil
}

type fakeOnboardProducts struct{ products map[string]ProductInfo }

func (f fakeOnboardProducts) Get(_ context.Context, _ coretenant.Scope, id string) (ProductInfo, error) {
	p, ok := f.products[id]
	if !ok {
		return ProductInfo{}, mikrotik.ErrProductNotFound
	}
	return p, nil
}

func (f fakeOnboardProducts) ListRecurringInUse(context.Context, coretenant.Scope) ([]ProductInfo, error) {
	return nil, nil
}

type onboardEnv struct {
	svc     *SecretOnboardingService
	secrets *fakeOnboardSecrets
	crm     *fakeOnboardCRM
	store   *memOnboardStore
	perms   *fakeOnboardPerms
}

func newOnboardEnv(t *testing.T) onboardEnv {
	t.Helper()
	secrets := &fakeOnboardSecrets{secrets: map[string]domain.PPPSecret{
		"s1": {ID: "s1", Name: "budi@pppoe", Present: true},
		"s2": {ID: "s2", Name: "budi-2@pppoe", Present: true},
	}}
	crm := newFakeCRM()
	store := &memOnboardStore{state: map[string]OnboardState{}}
	perms := &fakeOnboardPerms{}
	products := fakeOnboardProducts{products: map[string]ProductInfo{
		"p-10m":  {ID: "p-10m", Name: "Internet 10M", Recurring: true, Active: true, BasePrice: "200000"},
		"p-once": {ID: "p-once", Name: "Instalasi", Recurring: false, Active: true, BasePrice: "500000"},
	}}
	svc := NewSecretOnboardingService(secrets, crm, products, perms, store, &mutexLocker{held: map[string]bool{}})
	// 2026-11-12 10:00 WIB.
	svc.now = func() time.Time { return time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC) }
	return onboardEnv{svc: svc, secrets: secrets, crm: crm, store: store, perms: perms}
}

func newCustomerInput(secretID string) OnboardInput {
	return OnboardInput{
		SecretID: secretID, ProductID: "p-10m", BillingDay: 25,
		Customer: CustomerChoice{New: &NewContact{Name: "Sari", Phone: "0899 1111 2222", Email: "sari@b.com"}},
	}
}

func isCode(err error, code string) bool {
	var appErr *coreerrors.AppError
	return errors.As(err, &appErr) && appErr.Code == code
}

// ---- tests ----

func TestRun_HappyPath_NoInitialInvoice(t *testing.T) {
	e := newOnboardEnv(t)
	st, err := e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []string{"create_contact", "create_deal", "create_quotation", "approve_quotation", "ensure_sales_order", "confirm_sales_order"}
	if got := e.crm.writes(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("write order = %v, want %v", got, want)
	}
	if e.crm.deal.Source != OnboardingDealSource || e.crm.deal.ContactID != "c-new" || e.crm.deal.Value != "200000.00" {
		t.Fatalf("deal = %+v", e.crm.deal)
	}
	if q := e.crm.quotation; q.ProductID != "p-10m" || q.Quantity != "1" || q.UnitPrice != "" || q.ContactID != "c-new" {
		t.Fatalf("quotation = %+v (empty unit price = catalog price)", q)
	}
	if c := e.crm.confirm; !c.SkipInitialInvoice || c.StartDate.Format("2006-01-02") != "2026-10-25" || strings.Join(c.Channels, ",") != "email" {
		t.Fatalf("confirm = %+v", c)
	}
	if st.Stage != StagePaired || st.ContractID == "" || st.AccountID == "" || st.LastError != "" {
		t.Fatalf("state = %+v", st)
	}
	if s := e.secrets.secrets["s1"]; s.AccountID == nil || *s.AccountID != st.AccountID {
		t.Fatalf("secret not paired to the SO account: %+v", s)
	}
	// Repeating a finished onboarding changes nothing.
	again, err := e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1")
	if err != nil || again.ContractID != st.ContractID || e.crm.count("create_contact") != 1 || e.secrets.pairs != 1 {
		t.Fatalf("repeat: %+v err=%v", again, err)
	}
}

func TestRun_InitialInvoiceTickedKeepsNormalFlow(t *testing.T) {
	e := newOnboardEnv(t)
	in := newCustomerInput("s1")
	in.CreateInitialInvoice = true
	if _, err := e.svc.Run(ctx, testScope(t), in, "u1"); err != nil {
		t.Fatal(err)
	}
	if e.crm.confirm.SkipInitialInvoice {
		t.Fatal("ticked initial invoice must not skip it")
	}
}

func TestRun_PriceOverrideFlowsToQuotation(t *testing.T) {
	e := newOnboardEnv(t)
	in := newCustomerInput("s1")
	in.UnitPrice, in.Quantity = "150000", "2"
	if _, err := e.svc.Run(ctx, testScope(t), in, "u1"); err != nil {
		t.Fatal(err)
	}
	if e.crm.quotation.UnitPrice != "150000" || e.crm.quotation.Quantity != "2" || e.crm.deal.Value != "300000.00" {
		t.Fatalf("quotation = %+v deal = %+v", e.crm.quotation, e.crm.deal)
	}
	for _, bad := range []OnboardInput{
		{SecretID: "s2", ProductID: "p-10m", BillingDay: 5, UnitPrice: "15.000", Customer: CustomerChoice{ExistingContactID: "c-old"}},
		{SecretID: "s2", ProductID: "p-10m", BillingDay: 5, Quantity: "0", Customer: CustomerChoice{ExistingContactID: "c-old"}},
		{SecretID: "s2", ProductID: "p-10m", BillingDay: 32, Customer: CustomerChoice{ExistingContactID: "c-old"}},
		{SecretID: "s2", ProductID: "p-once", BillingDay: 5, Customer: CustomerChoice{ExistingContactID: "c-old"}},
	} {
		if _, err := e.svc.Run(ctx, testScope(t), bad, "u1"); !errors.Is(err, mikrotik.ErrInvalidInput) {
			t.Errorf("input %+v: err = %v, want ErrInvalidInput", bad, err)
		}
	}
}

func TestRun_IdempotentAndResumable(t *testing.T) {
	e := newOnboardEnv(t)
	e.crm.failOnce["create_quotation"] = errors.New("db down")

	st, err := e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1")
	if err == nil || st.Stage != StageDeal || st.LastError == "" || st.DealID == "" {
		t.Fatalf("first run: state = %+v err = %v", st, err)
	}
	if saved, _, _ := e.store.Get(ctx, testScope(t), "s1"); saved.Stage != StageDeal || saved.LastError == "" {
		t.Fatalf("stored state = %+v", saved)
	}

	st, err = e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1")
	if err != nil || st.Stage != StagePaired || st.LastError != "" {
		t.Fatalf("resume: state = %+v err = %v", st, err)
	}
	if e.crm.count("create_contact") != 1 || e.crm.count("create_deal") != 1 {
		t.Fatalf("resume duplicated writes: %v", e.crm.calls)
	}
}

func TestRun_ParallelRunsCreateOneContact(t *testing.T) {
	e := newOnboardEnv(t)
	e.crm.slow = 50 * time.Millisecond
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1")
		}()
	}
	wg.Wait()
	if e.crm.count("create_contact") != 1 {
		t.Fatalf("create_contact called %d times", e.crm.count("create_contact"))
	}
	inProgress := 0
	for _, err := range errs {
		if errors.Is(err, mikrotik.ErrOnboardingInProgress) {
			inProgress++
		} else if err != nil {
			t.Fatalf("unexpected error %v", err)
		}
	}
	if inProgress != 1 {
		t.Fatalf("errors = %v, want exactly one in-progress refusal", errs)
	}
}

func TestRun_DuplicateContactRejectedBeforeAnyWrite(t *testing.T) {
	e := newOnboardEnv(t)
	in := newCustomerInput("s1")
	in.Customer.New.Email = " BUDI@b.com"
	_, err := e.svc.Run(ctx, testScope(t), in, "u1")
	var dup *OnboardDuplicateError
	if !errors.As(err, &dup) || dup.Existing.ID != "c-old" {
		t.Fatalf("err = %v, want OnboardDuplicateError(c-old)", err)
	}
	if w := e.crm.writes(); len(w) != 0 {
		t.Fatalf("writes before duplicate check: %v", w)
	}
	if _, ok, _ := e.store.Get(ctx, testScope(t), "s1"); ok {
		t.Fatal("no state may be stored for a refused onboarding")
	}

	plan, err := e.svc.Preview(ctx, testScope(t), in, "u1")
	if err != nil || plan.DuplicateOf == nil || plan.DuplicateOf.ID != "c-old" {
		t.Fatalf("Preview duplicate = %+v err=%v", plan, err)
	}
}

func TestRun_PermissionMissingRejectedBeforeAnyWrite(t *testing.T) {
	e := newOnboardEnv(t)
	e.perms.missing = []string{"sales_order.confirm"}
	_, err := e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1")
	if !errors.Is(err, mikrotik.ErrOnboardingForbidden) || !strings.Contains(err.Error(), "sales_order.confirm") {
		t.Fatalf("err = %v, want ErrOnboardingForbidden naming the permission", err)
	}
	if w := e.crm.writes(); len(w) != 0 {
		t.Fatalf("writes: %v", w)
	}
}

func TestRun_QuotaExceededStopsWithResumableStage(t *testing.T) {
	e := newOnboardEnv(t)
	e.crm.createErr = coreerrors.New("QUOTA_EXCEEDED", "Kuota kontak penuh.", http.StatusConflict)

	st, err := e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1")
	if !isCode(err, "QUOTA_EXCEEDED") || st.Stage != StageNone || st.LastError != "Kuota kontak penuh." {
		t.Fatalf("state = %+v err = %v", st, err)
	}
	e.crm.createErr = nil
	st, err = e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1")
	if err != nil || st.Stage != StagePaired {
		t.Fatalf("after quota freed: %+v err=%v", st, err)
	}
}

func TestRun_InternalErrorMessageIsGeneric(t *testing.T) {
	e := newOnboardEnv(t)
	e.crm.failOnce["create_deal"] = errors.New("pq: password authentication failed for user x")
	st, err := e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1")
	if err == nil || strings.Contains(st.LastError, "password") || st.LastError == "" {
		t.Fatalf("LastError = %q (must not leak internals)", st.LastError)
	}
}

func TestRun_PairedSecretWithContractIsRejected(t *testing.T) {
	e := newOnboardEnv(t)
	acct := "acct-legacy"
	s := e.secrets.secrets["s1"]
	s.AccountID = &acct
	e.secrets.secrets["s1"] = s
	e.crm.accountOf[acct] = "c-old"
	e.crm.active[acct] = true

	if _, err := e.svc.Run(ctx, testScope(t), OnboardInput{SecretID: "s1", ProductID: "p-10m", BillingDay: 5}, "u1"); !errors.Is(err, mikrotik.ErrSecretHasContract) {
		t.Fatalf("err = %v, want ErrSecretHasContract", err)
	}
	if w := e.crm.writes(); len(w) != 0 {
		t.Fatalf("writes: %v", w)
	}
}

func TestRun_PairedSecretWithoutContractCompletes(t *testing.T) {
	e := newOnboardEnv(t)
	acct := "acct-so-quo-deal-c-old" // what the fake confirm returns for contact c-old
	s := e.secrets.secrets["s1"]
	s.AccountID = &acct
	e.secrets.secrets["s1"] = s
	e.crm.accountOf[acct] = "c-old"

	// The customer choice is ignored: a paired secret keeps its account's contact.
	in := newCustomerInput("s1")
	st, err := e.svc.Run(ctx, testScope(t), in, "u1")
	if err != nil || st.Stage != StagePaired || st.ContactID != "c-old" {
		t.Fatalf("state = %+v err = %v", st, err)
	}
	if e.crm.count("create_contact") != 0 || e.secrets.pairs != 0 {
		t.Fatalf("must reuse contact and the existing pairing: %v pairs=%d", e.crm.calls, e.secrets.pairs)
	}
}

func TestRun_SecondSecretSameContactCreatesOwnDealAndContract(t *testing.T) {
	e := newOnboardEnv(t)
	first, err := e.svc.Run(ctx, testScope(t), OnboardInput{SecretID: "s1", ProductID: "p-10m", BillingDay: 5, Customer: CustomerChoice{ExistingContactID: "c-old"}}, "u1")
	if err != nil {
		t.Fatal(err)
	}
	e.crm.mu.Lock()
	e.crm.calls = nil
	e.crm.mu.Unlock()
	// The fake derives ids from their parents, so give the second deal a distinct id.
	second, err := e.svc.Run(ctx, testScope(t), OnboardInput{SecretID: "s2", ProductID: "p-10m", BillingDay: 5, Customer: CustomerChoice{ExistingContactID: "c-old"}}, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if e.crm.count("create_deal") != 1 || e.crm.count("confirm_sales_order") != 1 || e.crm.count("create_contact") != 0 {
		t.Fatalf("second secret calls = %v", e.crm.calls)
	}
	if first.ContactID != second.ContactID || first.SecretID == second.SecretID {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestPreview_PlanWithoutWrites(t *testing.T) {
	e := newOnboardEnv(t)
	plan, err := e.svc.Preview(ctx, testScope(t), OnboardInput{SecretID: "s1", ProductID: "p-10m", BillingDay: 31, Customer: CustomerChoice{ExistingContactID: "c-old"}}, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if plan.ContactAction != "existing" || plan.Contact.ID != "c-old" || plan.InitialInvoice ||
		plan.ContractStartDate.Format("2006-01-02") != "2026-10-31" || plan.UnitPrice != "200000" || plan.DealTitle == "" {
		t.Fatalf("plan = %+v", plan)
	}
	if w := e.crm.writes(); len(w) != 0 {
		t.Fatalf("Preview wrote: %v", w)
	}
}

func TestRun_ContactWithoutEmailOrPhoneIsRejected(t *testing.T) {
	e := newOnboardEnv(t)
	in := newCustomerInput("s1")
	in.Customer.New = &NewContact{Name: "Tanpa Kontak"}
	if _, err := e.svc.Run(ctx, testScope(t), in, "u1"); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput (an invoice needs email or WhatsApp)", err)
	}
}

func TestGetSecret_IncludesOnboardingAndContract(t *testing.T) {
	e := newOnboardEnv(t)
	d, err := e.svc.Detail(ctx, testScope(t), "s1")
	if err != nil || d.Onboarding != nil || d.Contract != nil || d.Secret.ID != "s1" || !d.CanOnboard {
		t.Fatalf("fresh detail = %+v err=%v", d, err)
	}
	if _, err := e.svc.Run(ctx, testScope(t), newCustomerInput("s1"), "u1"); err != nil {
		t.Fatal(err)
	}
	d, err = e.svc.Detail(ctx, testScope(t), "s1")
	if err != nil || d.Onboarding == nil || d.Onboarding.Stage != StagePaired || d.Contract == nil || d.Contract.Number != "CTR-ctr-so-quo-deal-c-new" || d.CanOnboard {
		t.Fatalf("onboarded detail = %+v err=%v", d, err)
	}

	// A legacy pairing with an active contract shows that contract and blocks onboarding.
	acct := "acct-legacy"
	s := e.secrets.secrets["s2"]
	s.AccountID = &acct
	e.secrets.secrets["s2"] = s
	e.crm.active[acct] = true
	d, err = e.svc.Detail(ctx, testScope(t), "s2")
	if err != nil || d.Contract == nil || d.Contract.ID != "ctr-of-"+acct || d.CanOnboard {
		t.Fatalf("legacy detail = %+v err=%v", d, err)
	}
}
