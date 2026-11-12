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

	"zyad.cloud/internal/core/businesstime"
	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/shared/pricing"
)

// OnboardingDealSource marks deals created by the secret onboarding. It must
// equal crm domain.DealSourcePPPoEImport (the app adapter maps it 1:1).
const OnboardingDealSource = "Impor PPPoE"

// Onboarding stages, in order. A state's Stage is the last completed one;
// StageNone means nothing has been created yet.
const (
	StageNone       = ""
	StageContact    = "contact"
	StageDeal       = "deal"
	StageQuotation  = "quotation"
	StageSalesOrder = "sales_order"
	StageConfirmed  = "confirmed"
	StagePaired     = "paired"
)

var stageOrder = map[string]int{
	StageNone: 0, StageContact: 1, StageDeal: 2, StageQuotation: 3, StageSalesOrder: 4, StageConfirmed: 5, StagePaired: 6,
}

// stageAtLeast reports whether stage s is st or later.
func stageAtLeast(s, st string) bool { return stageOrder[s] >= stageOrder[st] }

// OnboardContactRef is the CRM contact view the onboarding needs.
type OnboardContactRef struct{ ID, Name, Email, Phone string }

// NewContact is a contact the onboarding creates.
type NewContact struct{ Name, Phone, Email string }

// CustomerChoice: an existing CRM contact, or a new one. Ignored for a secret
// already paired to an account (its account's contact is used).
type CustomerChoice struct {
	ExistingContactID string
	New               *NewContact
}

// OnboardInput is one onboarding request. Quantity "" = 1; UnitPrice "" =
// catalog price. BillingDay is "tanggal tagih setiap bulan" (1–31). Channels
// "" = email when the contact has one, else WhatsApp.
type OnboardInput struct {
	SecretID             string
	Customer             CustomerChoice
	ProductID            string
	Quantity, UnitPrice  string
	BillingDay           int
	CreateInitialInvoice bool
	Channels             []string
}

// OnboardPlan is what Run would create (Preview, no writes).
type OnboardPlan struct {
	SecretName          string
	ContactAction       string // "existing" | "new"
	Contact             OnboardContactRef
	ProductName         string
	Quantity, UnitPrice string
	DealTitle           string
	ContractStartDate   time.Time
	InitialInvoice      bool
	Channels            []string
	// DuplicateOf is set when the new contact would duplicate this one; Run
	// refuses that input and the UI offers the existing contact instead.
	DuplicateOf *OnboardContactRef
}

// OnboardState is the persisted progress of one secret's onboarding.
type OnboardState struct {
	SecretID                                                 string
	Stage                                                    string
	ContactID, DealID, QuotationID, SalesOrderID, ContractID string
	AccountID                                                string
	LastError                                                string
	UpdatedAt                                                time.Time
}

// OnboardDeal / OnboardQuotation / ConfirmOrder are the CRM writes.
type OnboardDeal struct{ ContactID, Title, Value, Source string }
type OnboardQuotation struct{ DealID, ContactID, ProductID, Quantity, UnitPrice string }
type ConfirmOrder struct {
	StartDate          time.Time
	SkipInitialInvoice bool
	Channels           []string
}

// ContractRef identifies a receivable contract.
type ContractRef struct{ ID, Number string }

// ConfirmedOrder is the billing outcome of a confirmed sales order.
type ConfirmedOrder struct{ AccountID, ContractID string }

// OnboardingCRM runs the existing CRM/receivable chain (adapted in
// internal/app). Every write must be safe to call again for the same parent
// after a failure: ApproveQuotation / EnsureSalesOrder / ConfirmSalesOrder are
// idempotent per id, and Create* are only called once their stage is missing.
type OnboardingCRM interface {
	// FindDuplicateContact: live contact sharing the normalized email or phone.
	FindDuplicateContact(ctx context.Context, scope coretenant.Scope, email, phone string) (OnboardContactRef, bool, error)
	// GetContact returns mikrotik.ErrContactNotFound when absent.
	GetContact(ctx context.Context, scope coretenant.Scope, id string) (OnboardContactRef, error)
	// CreateContact goes through the CRM service (duplicate guard + quota).
	CreateContact(ctx context.Context, scope coretenant.Scope, c NewContact, actor string) (OnboardContactRef, error)
	// ContactOfAccount: the CRM contact a receivable account was created for ("" = none).
	ContactOfAccount(ctx context.Context, scope coretenant.Scope, accountID string) (string, error)
	AccountHasActiveContract(ctx context.Context, scope coretenant.Scope, accountID string) (bool, error)
	// ContractRef reads one contract; ActiveContractOfAccount the account's
	// first active one (found=false when none).
	ContractRef(ctx context.Context, scope coretenant.Scope, id string) (ContractRef, error)
	ActiveContractOfAccount(ctx context.Context, scope coretenant.Scope, accountID string) (ContractRef, bool, error)
	CreateDeal(ctx context.Context, scope coretenant.Scope, d OnboardDeal, actor string) (string, error)
	CreateQuotation(ctx context.Context, scope coretenant.Scope, q OnboardQuotation, actor string) (string, error)
	// ApproveQuotation marks a draft sent and approves it; approved = no-op.
	ApproveQuotation(ctx context.Context, scope coretenant.Scope, quotationID, actor string) error
	// EnsureSalesOrder returns the SO of an approved quotation, creating it when the approve hook did not.
	EnsureSalesOrder(ctx context.Context, scope coretenant.Scope, quotationID, actor string) (string, error)
	// ConfirmSalesOrder fills a draft and confirms it, or retries failed
	// billing of a confirmed one; it fails unless a contract exists afterwards.
	ConfirmSalesOrder(ctx context.Context, scope coretenant.Scope, soID string, o ConfirmOrder, actor string) (ConfirmedOrder, error)
}

// OnboardingPermissions lists the permissions the operator lacks for the
// chain (contact create only when newContact).
type OnboardingPermissions interface {
	Missing(ctx context.Context, scope coretenant.Scope, actor string, newContact bool) ([]string, error)
}

// OnboardingStore persists OnboardState per secret.
type OnboardingStore interface {
	Get(ctx context.Context, scope coretenant.Scope, secretID string) (OnboardState, bool, error)
	Save(ctx context.Context, scope coretenant.Scope, s OnboardState, actor string) error
}

// OnboardingSecrets is the secret side; satisfied by *SecretService.
type OnboardingSecrets interface {
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.PPPSecret, error)
	// CheckOnboardable: the secret could be paired (present, profile known);
	// an existing pairing is allowed.
	CheckOnboardable(ctx context.Context, scope coretenant.Scope, s domain.PPPSecret) error
	Pair(ctx context.Context, scope coretenant.Scope, secretID, accountID, actor string) (domain.PPPSecret, error)
}

// OnboardDuplicateError: the new contact's email/WhatsApp already belongs to Existing.
type OnboardDuplicateError struct{ Existing OnboardContactRef }

func (e *OnboardDuplicateError) Error() string { return "contact with the same email or WhatsApp already exists" }

// SecretOnboardingService turns a PPP secret into a billed customer through
// the existing chain contact → deal → quotation → sales order → contract, then
// pairs the secret. Idempotent and resumable per secret: each completed stage
// is stored, a retry continues after the last one, and a per-secret advisory
// lock keeps two runs from racing.
type SecretOnboardingService struct {
	secrets  OnboardingSecrets
	crm      OnboardingCRM
	products ProductCatalog
	perms    OnboardingPermissions
	store    OnboardingStore
	locker   SetupLocker
	now      func() time.Time
}

func NewSecretOnboardingService(secrets OnboardingSecrets, crm OnboardingCRM, products ProductCatalog,
	perms OnboardingPermissions, store OnboardingStore, locker SetupLocker) *SecretOnboardingService {
	return &SecretOnboardingService{secrets: secrets, crm: crm, products: products, perms: perms, store: store, locker: locker, now: time.Now}
}

var decimalPattern = regexp.MustCompile(`^\d{1,13}(\.\d{1,2})?$`)

func onboardInvalid(msg string) error { return &mikrotik.InvalidInputError{Message: msg} }

// resolved is the validated input plus what was read to validate it.
type resolved struct {
	secret      domain.PPPSecret
	product     ProductInfo
	contact     OnboardContactRef // empty ID when a new contact will be created
	newContact  *NewContact
	duplicateOf *OnboardContactRef
	quantity    string
	start       time.Time
	channels    []string
}

// State returns the stored onboarding state of a secret (found=false when none).
func (s *SecretOnboardingService) State(ctx context.Context, scope coretenant.Scope, secretID string) (OnboardState, bool, error) {
	return s.store.Get(ctx, scope, secretID)
}

// SecretDetail is the secret page: the secret, its onboarding progress and
// its contract (the onboarding's, else for a legacy pairing the paired
// account's active one). CanOnboard is false once a contract exists or the
// secret cannot be paired.
type SecretDetail struct {
	Secret     domain.PPPSecret
	Onboarding *OnboardState
	Contract   *ContractRef
	CanOnboard bool
}

// Detail assembles SecretDetail without writing.
func (s *SecretOnboardingService) Detail(ctx context.Context, scope coretenant.Scope, secretID string) (SecretDetail, error) {
	secret, err := s.secrets.Get(ctx, scope, secretID)
	if err != nil {
		return SecretDetail{}, err
	}
	d := SecretDetail{Secret: secret}
	state, found, err := s.store.Get(ctx, scope, secretID)
	if err != nil {
		return SecretDetail{}, err
	}
	if found {
		d.Onboarding = &state
	}
	switch {
	case found && state.ContractID != "":
		c, err := s.crm.ContractRef(ctx, scope, state.ContractID)
		if err != nil {
			return SecretDetail{}, err
		}
		d.Contract = &c
	case !found && secret.AccountID != nil:
		c, ok, err := s.crm.ActiveContractOfAccount(ctx, scope, *secret.AccountID)
		if err != nil {
			return SecretDetail{}, err
		}
		if ok {
			d.Contract = &c
		}
	}
	d.CanOnboard = d.Contract == nil && s.secrets.CheckOnboardable(ctx, scope, secret) == nil
	return d, nil
}

// Preview validates the input and describes what Run would create, without
// writing. A duplicate new contact is reported in DuplicateOf, not as an error.
func (s *SecretOnboardingService) Preview(ctx context.Context, scope coretenant.Scope, in OnboardInput, actor string) (OnboardPlan, error) {
	state, _, err := s.store.Get(ctx, scope, in.SecretID)
	if err != nil {
		return OnboardPlan{}, err
	}
	r, err := s.resolve(ctx, scope, in, state, actor)
	if err != nil {
		return OnboardPlan{}, err
	}
	plan := OnboardPlan{
		SecretName: r.secret.Name, ProductName: r.product.Name, Quantity: r.quantity,
		UnitPrice: orString(strings.TrimSpace(in.UnitPrice), r.product.BasePrice), DealTitle: dealTitle(r.product, r.secret),
		ContractStartDate: r.start, InitialInvoice: in.CreateInitialInvoice, Channels: r.channels, DuplicateOf: r.duplicateOf,
		ContactAction: "existing", Contact: r.contact,
	}
	if r.newContact != nil {
		plan.ContactAction = "new"
		plan.Contact = OnboardContactRef{Name: r.newContact.Name, Email: r.newContact.Email, Phone: r.newContact.Phone}
	}
	return plan, nil
}

// Run executes (or resumes) the onboarding. On failure the state keeps the
// last completed stage and a user-facing LastError, and the error is returned.
func (s *SecretOnboardingService) Run(ctx context.Context, scope coretenant.Scope, in OnboardInput, actor string) (OnboardState, error) {
	unlock, ok, err := s.locker.TryLock(ctx, "mikrotik.onboard:"+scope.OrganizationID()+":"+in.SecretID)
	if err != nil {
		return OnboardState{}, err
	}
	if !ok {
		return OnboardState{}, mikrotik.ErrOnboardingInProgress
	}
	defer unlock()

	state, found, err := s.store.Get(ctx, scope, in.SecretID)
	if err != nil {
		return OnboardState{}, err
	}
	if found && state.Stage == StagePaired {
		return state, nil
	}
	r, err := s.resolve(ctx, scope, in, state, actor)
	if err != nil {
		return state, err
	}
	if r.duplicateOf != nil {
		return state, &OnboardDuplicateError{Existing: *r.duplicateOf}
	}
	state.SecretID = in.SecretID
	if err := s.advance(ctx, scope, &state, in, r, actor); err != nil {
		state.LastError = userMessage(err)
		if saveErr := s.store.Save(ctx, scope, state, actor); saveErr != nil {
			slog.ErrorContext(ctx, "mikrotik onboarding: save failure state", "secret_id", in.SecretID, "error", saveErr)
		}
		return state, err
	}
	return state, nil
}

// advance runs the missing stages in order, saving after each one.
func (s *SecretOnboardingService) advance(ctx context.Context, scope coretenant.Scope, st *OnboardState, in OnboardInput, r resolved, actor string) error {
	save := func(stage string) error {
		st.Stage, st.LastError = stage, ""
		return s.store.Save(ctx, scope, *st, actor)
	}
	if !stageAtLeast(st.Stage, StageContact) {
		if r.newContact != nil {
			c, err := s.crm.CreateContact(ctx, scope, *r.newContact, actor)
			if err != nil {
				return err
			}
			r.contact = c
		}
		st.ContactID = r.contact.ID
		if err := save(StageContact); err != nil {
			return err
		}
	}
	if !stageAtLeast(st.Stage, StageDeal) {
		unitPrice := orString(strings.TrimSpace(in.UnitPrice), r.product.BasePrice)
		id, err := s.crm.CreateDeal(ctx, scope, OnboardDeal{
			ContactID: st.ContactID, Title: dealTitle(r.product, r.secret), Value: lineValue(r.quantity, unitPrice), Source: OnboardingDealSource,
		}, actor)
		if err != nil {
			return err
		}
		st.DealID = id
		if err := save(StageDeal); err != nil {
			return err
		}
	}
	if !stageAtLeast(st.Stage, StageQuotation) {
		id, err := s.crm.CreateQuotation(ctx, scope, OnboardQuotation{
			DealID: st.DealID, ContactID: st.ContactID, ProductID: in.ProductID, Quantity: r.quantity, UnitPrice: strings.TrimSpace(in.UnitPrice),
		}, actor)
		if err != nil {
			return err
		}
		st.QuotationID = id
		if err := save(StageQuotation); err != nil {
			return err
		}
	}
	if !stageAtLeast(st.Stage, StageSalesOrder) {
		if err := s.crm.ApproveQuotation(ctx, scope, st.QuotationID, actor); err != nil {
			return err
		}
		id, err := s.crm.EnsureSalesOrder(ctx, scope, st.QuotationID, actor)
		if err != nil {
			return err
		}
		st.SalesOrderID = id
		if err := save(StageSalesOrder); err != nil {
			return err
		}
	}
	if !stageAtLeast(st.Stage, StageConfirmed) {
		out, err := s.crm.ConfirmSalesOrder(ctx, scope, st.SalesOrderID, ConfirmOrder{
			StartDate: r.start, SkipInitialInvoice: !in.CreateInitialInvoice, Channels: r.channels,
		}, actor)
		if err != nil {
			return err
		}
		st.AccountID, st.ContractID = out.AccountID, out.ContractID
		if err := save(StageConfirmed); err != nil {
			return err
		}
	}
	secret, err := s.secrets.Get(ctx, scope, in.SecretID)
	if err != nil {
		return err
	}
	switch {
	case secret.AccountID == nil:
		if _, err := s.secrets.Pair(ctx, scope, secret.ID, st.AccountID, actor); err != nil {
			return err
		}
	case *secret.AccountID != st.AccountID:
		return mikrotik.ErrSecretAlreadyPaired
	}
	return save(StagePaired)
}

// resolve validates the input before anything is written. Stages already
// completed in state are not re-validated against the (possibly changed) input.
func (s *SecretOnboardingService) resolve(ctx context.Context, scope coretenant.Scope, in OnboardInput, state OnboardState, actor string) (resolved, error) {
	var r resolved
	secret, err := s.secrets.Get(ctx, scope, in.SecretID)
	if err != nil {
		return r, err
	}
	if err := s.secrets.CheckOnboardable(ctx, scope, secret); err != nil {
		return r, err
	}
	r.secret = secret
	if in.BillingDay < 1 || in.BillingDay > 31 {
		return r, onboardInvalid("Tanggal tagih setiap bulan harus 1–31.")
	}
	r.start = pricing.CurrentCycleStart(in.BillingDay, businesstime.DayOf(s.now()))
	r.quantity = orString(strings.TrimSpace(in.Quantity), "1")
	if !decimalPattern.MatchString(r.quantity) || isZero(r.quantity) {
		return r, onboardInvalid("Jumlah harus angka lebih dari 0 (desimal pakai titik).")
	}
	if price := strings.TrimSpace(in.UnitPrice); price != "" && !decimalPattern.MatchString(price) {
		return r, onboardInvalid("Harga harus angka tanpa pemisah ribuan (desimal pakai titik).")
	}
	product, err := s.products.Get(ctx, scope, in.ProductID)
	if err != nil {
		return r, err
	}
	if !stageAtLeast(state.Stage, StageQuotation) && (!product.Active || !product.Recurring) {
		return r, onboardInvalid("Pilih produk aktif yang ditagih berulang.")
	}
	r.product = product

	newContact, err := s.resolveCustomer(ctx, scope, in, state, &r)
	if err != nil {
		return r, err
	}
	missing, err := s.perms.Missing(ctx, scope, actor, newContact)
	if err != nil {
		return r, err
	}
	if len(missing) > 0 {
		return r, fmt.Errorf("%w: %s", mikrotik.ErrOnboardingForbidden, strings.Join(missing, ", "))
	}
	email, phone := r.contact.Email, r.contact.Phone
	if r.newContact != nil {
		email, phone = r.newContact.Email, r.newContact.Phone
	}
	r.channels, err = resolveChannels(in.Channels, email, phone)
	return r, err
}

// resolveCustomer picks the contact: the stored one when resuming, the paired
// account's contact for a paired secret, else the operator's choice. It
// reports whether a new contact will be created.
func (s *SecretOnboardingService) resolveCustomer(ctx context.Context, scope coretenant.Scope, in OnboardInput, state OnboardState, r *resolved) (bool, error) {
	if state.ContactID != "" {
		c, err := s.crm.GetContact(ctx, scope, state.ContactID)
		r.contact = c
		return false, err
	}
	if r.secret.AccountID != nil {
		accountID := *r.secret.AccountID
		if state.Stage == StageNone {
			has, err := s.crm.AccountHasActiveContract(ctx, scope, accountID)
			if err != nil {
				return false, err
			}
			if has {
				return false, mikrotik.ErrSecretHasContract
			}
		}
		contactID, err := s.crm.ContactOfAccount(ctx, scope, accountID)
		if err != nil {
			return false, err
		}
		if contactID == "" {
			return false, onboardInvalid("Akun pelanggan secret ini tidak terhubung ke kontak CRM. Lepas pasangan secret lalu onboard ulang.")
		}
		c, err := s.crm.GetContact(ctx, scope, contactID)
		r.contact = c
		return false, err
	}
	switch {
	case in.Customer.ExistingContactID != "":
		c, err := s.crm.GetContact(ctx, scope, in.Customer.ExistingContactID)
		r.contact = c
		return false, err
	case in.Customer.New != nil:
		nc := NewContact{
			Name: strings.TrimSpace(in.Customer.New.Name), Email: strings.TrimSpace(in.Customer.New.Email), Phone: strings.TrimSpace(in.Customer.New.Phone),
		}
		if n := len([]rune(nc.Name)); n < 2 || n > 200 {
			return false, onboardInvalid("Nama pelanggan wajib diisi (2–200 karakter).")
		}
		dup, found, err := s.crm.FindDuplicateContact(ctx, scope, nc.Email, nc.Phone)
		if err != nil {
			return false, err
		}
		if found {
			r.duplicateOf = &dup
		}
		r.newContact = &nc
		return true, nil
	}
	return false, onboardInvalid("Pilih kontak yang sudah ada atau isi data pelanggan baru.")
}

// resolveChannels: requested channels must be usable for the contact;
// default = email when there is one, else WhatsApp.
func resolveChannels(requested []string, email, phone string) ([]string, error) {
	if len(requested) == 0 {
		switch {
		case strings.TrimSpace(email) != "":
			return []string{"email"}, nil
		case strings.TrimSpace(phone) != "":
			return []string{"whatsapp"}, nil
		}
		return nil, onboardInvalid("Isi email atau nomor WhatsApp pelanggan agar invoice bisa dikirim.")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, c := range requested {
		switch {
		case c == "email" && strings.TrimSpace(email) == "":
			return nil, onboardInvalid("Kanal email butuh alamat email pelanggan.")
		case c == "whatsapp" && strings.TrimSpace(phone) == "":
			return nil, onboardInvalid("Kanal WhatsApp butuh nomor WhatsApp pelanggan.")
		case c != "email" && c != "whatsapp":
			return nil, onboardInvalid("Kanal harus email atau whatsapp.")
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}

func dealTitle(p ProductInfo, s domain.PPPSecret) string {
	title := p.Name + " - " + s.Name
	if r := []rune(title); len(r) > 200 {
		title = string(r[:200])
	}
	return title
}

// lineValue = quantity × unit price, 2 decimals ("0.00" when unparsable).
func lineValue(quantity, unitPrice string) string {
	q, okQ := new(big.Rat).SetString(quantity)
	p, okP := new(big.Rat).SetString(orString(unitPrice, "0"))
	if !okQ || !okP {
		return "0.00"
	}
	return new(big.Rat).Mul(q, p).FloatString(2)
}

func isZero(v string) bool {
	r, ok := new(big.Rat).SetString(v)
	return !ok || r.Sign() == 0
}

func orString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// userMessage is the LastError shown to the operator: deliberate 4xx
// refusals keep their text, anything else is generic (no internals).
func userMessage(err error) string {
	var appErr *coreerrors.AppError
	var invalidErr *mikrotik.InvalidInputError
	switch {
	case errors.As(err, &appErr) && appErr.Status >= 400 && appErr.Status < 500:
		return appErr.Message
	case errors.As(err, &invalidErr):
		return invalidErr.Message
	case errors.Is(err, mikrotik.ErrSecretAlreadyPaired):
		return "Secret sudah dipasangkan ke pelanggan lain."
	case errors.Is(err, mikrotik.ErrSecretNotPairable):
		return "Secret belum bisa dipasangkan (tidak ada di router atau profil asli belum diisi)."
	}
	return "Terjadi kesalahan saat memproses onboarding. Coba lagi; tahap yang sudah selesai tidak diulang."
}

// onboardingStore adapts *repository.SecretOnboardingRepository to OnboardingStore.
type onboardingStore struct{ repo *repository.SecretOnboardingRepository }

// NewOnboardingStore wraps the repository for SecretOnboardingService.
func NewOnboardingStore(repo *repository.SecretOnboardingRepository) OnboardingStore {
	return onboardingStore{repo: repo}
}

func (s onboardingStore) Get(ctx context.Context, scope coretenant.Scope, secretID string) (OnboardState, bool, error) {
	o, found, err := s.repo.Get(ctx, scope, secretID)
	return stateFromRow(o), found, err
}

func (s onboardingStore) Save(ctx context.Context, scope coretenant.Scope, st OnboardState, actor string) error {
	return s.repo.Save(ctx, scope, repository.SecretOnboarding{
		SecretID: st.SecretID, Stage: st.Stage, ContactID: st.ContactID, DealID: st.DealID, QuotationID: st.QuotationID,
		SalesOrderID: st.SalesOrderID, ContractID: st.ContractID, AccountID: st.AccountID, LastError: st.LastError,
	}, actor)
}

func stateFromRow(o repository.SecretOnboarding) OnboardState {
	return OnboardState{
		SecretID: o.SecretID, Stage: o.Stage, ContactID: o.ContactID, DealID: o.DealID, QuotationID: o.QuotationID,
		SalesOrderID: o.SalesOrderID, ContractID: o.ContractID, AccountID: o.AccountID, LastError: o.LastError, UpdatedAt: o.UpdatedAt,
	}
}
