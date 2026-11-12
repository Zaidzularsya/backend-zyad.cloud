package app

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	crmservice "zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/modules/mikrotik"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivablerepo "zyad.cloud/internal/modules/receivable/repository"
)

// The secret onboarding (mikrotik) drives the existing CRM chain through this
// adapter: contact → deal → quotation → sales order → contract. Every step
// goes through the CRM services, so their validation, quotas, numbering,
// activities and the deal Won evaluation stay the single source of truth.

type onboardContactService interface {
	Create(ctx context.Context, scope coretenant.Scope, params crmrepo.CreateContactParams) (crmdomain.Contact, error)
	Get(ctx context.Context, scope coretenant.Scope, id string) (crmdomain.Contact, error)
}

type onboardContactFinder interface {
	FindDuplicate(ctx context.Context, scope coretenant.Scope, email, phoneNormalized, excludeID string) (crmdomain.Contact, bool, error)
}

type onboardDealService interface {
	Create(ctx context.Context, scope coretenant.Scope, params crmrepo.CreateDealParams) (crmdomain.Deal, error)
}

type onboardPipelines interface {
	List(ctx context.Context, scope coretenant.Scope, f crmrepo.PipelineListFilter) ([]crmdomain.Pipeline, int64, error)
}

type onboardQuotationService interface {
	Create(ctx context.Context, scope coretenant.Scope, in crmservice.CreateQuotationInput) (crmdomain.Quotation, error)
	Get(ctx context.Context, scope coretenant.Scope, id string) (crmdomain.Quotation, error)
	Send(ctx context.Context, scope coretenant.Scope, id, userID string) (crmdomain.Quotation, error)
	Approve(ctx context.Context, scope coretenant.Scope, id, userID string) (crmdomain.Quotation, error)
}

type onboardSalesOrderService interface {
	QuotationApproved(ctx context.Context, scope coretenant.Scope, q crmdomain.Quotation, actorUserID string) error
	Get(ctx context.Context, scope coretenant.Scope, id string) (crmdomain.SalesOrder, error)
	UpdateDraft(ctx context.Context, scope coretenant.Scope, id string, f crmrepo.SalesOrderDraftFields, userID string) (crmdomain.SalesOrder, error)
	Confirm(ctx context.Context, scope coretenant.Scope, id, userID string) (crmdomain.SalesOrder, error)
	RetryBilling(ctx context.Context, scope coretenant.Scope, id, userID string) (crmdomain.SalesOrder, error)
}

type onboardSalesOrderFinder interface {
	FindByQuotation(ctx context.Context, scope coretenant.Scope, quotationID string) (crmdomain.SalesOrder, error)
}

type onboardAccounts interface {
	Get(ctx context.Context, scope coretenant.Scope, id string) (receivabledomain.Account, error)
}

type onboardContracts interface {
	List(ctx context.Context, scope coretenant.Scope, f receivablerepo.ContractListFilter) ([]receivabledomain.Contract, int64, error)
}

type mikrotikOnboardingCRM struct {
	contacts    onboardContactService
	duplicates  onboardContactFinder
	deals       onboardDealService
	pipelines   onboardPipelines
	quotations  onboardQuotationService
	salesOrders onboardSalesOrderService
	soFinder    onboardSalesOrderFinder
	accounts    onboardAccounts
	contracts   onboardContracts
}

func contactRef(c crmdomain.Contact) mikrotikservice.OnboardContactRef {
	return mikrotikservice.OnboardContactRef{ID: c.ID, Name: strings.TrimSpace(c.FirstName + " " + c.LastName), Email: c.Email, Phone: c.Phone}
}

func onboardStepError(code, message string) error {
	return coreerrors.New(code, message, http.StatusConflict)
}

func (a mikrotikOnboardingCRM) FindDuplicateContact(ctx context.Context, scope coretenant.Scope, email, phone string) (mikrotikservice.OnboardContactRef, bool, error) {
	if e := crmservice.NormalizeEmail(email); e != "" {
		c, found, err := a.duplicates.FindDuplicate(ctx, scope, e, "", "")
		if err != nil || found {
			return contactRef(c), found, err
		}
	}
	if p := crmservice.NormalizePhone(phone); p != "" {
		c, found, err := a.duplicates.FindDuplicate(ctx, scope, "", p, "")
		return contactRef(c), found, err
	}
	return mikrotikservice.OnboardContactRef{}, false, nil
}

func (a mikrotikOnboardingCRM) GetContact(ctx context.Context, scope coretenant.Scope, id string) (mikrotikservice.OnboardContactRef, error) {
	c, err := a.contacts.Get(ctx, scope, id)
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) && appErr.Status == http.StatusNotFound {
		return mikrotikservice.OnboardContactRef{}, mikrotik.ErrContactNotFound
	}
	if err != nil {
		return mikrotikservice.OnboardContactRef{}, err
	}
	return contactRef(c), nil
}

func (a mikrotikOnboardingCRM) CreateContact(ctx context.Context, scope coretenant.Scope, in mikrotikservice.NewContact, actor string) (mikrotikservice.OnboardContactRef, error) {
	c, err := a.contacts.Create(ctx, scope, crmrepo.CreateContactParams{
		FirstName: in.Name, Email: in.Email, Phone: in.Phone, Source: mikrotikImportContactSource,
		Tags: []string{mikrotikImportContactTag}, IsCustomer: true, LifecycleStage: crmdomain.ContactLifecycleCustomer, CreatedBy: actor,
	})
	var dup *crmservice.DuplicateContactError
	if errors.As(err, &dup) { // lost a race with another create: offer the winner
		return mikrotikservice.OnboardContactRef{}, &mikrotikservice.OnboardDuplicateError{Existing: contactRef(dup.Existing)}
	}
	if err != nil {
		return mikrotikservice.OnboardContactRef{}, err
	}
	return contactRef(c), nil
}

func (a mikrotikOnboardingCRM) ContactOfAccount(ctx context.Context, scope coretenant.Scope, accountID string) (string, error) {
	acc, err := a.accounts.Get(ctx, scope, accountID)
	if err != nil {
		return "", err
	}
	return acc.ContactID(), nil
}

func (a mikrotikOnboardingCRM) AccountHasActiveContract(ctx context.Context, scope coretenant.Scope, accountID string) (bool, error) {
	_, total, err := a.contracts.List(ctx, scope, receivablerepo.ContractListFilter{
		Status: string(receivabledomain.ContractActive), AccountID: accountID, Limit: 1,
	})
	return total > 0, err
}

// defaultStage picks the default (else the most recently created) active pipeline and its
// first open stage, the same start a manual deal would use.
func (a mikrotikOnboardingCRM) defaultStage(ctx context.Context, scope coretenant.Scope) (pipelineID, stageID string, err error) {
	pipelines, _, err := a.pipelines.List(ctx, scope, crmrepo.PipelineListFilter{Limit: 100})
	if err != nil {
		return "", "", err
	}
	var chosen *crmdomain.Pipeline
	for i := range pipelines {
		p := &pipelines[i]
		if p.ArchivedAt != nil {
			continue
		}
		if chosen == nil || (p.IsDefault && !chosen.IsDefault) {
			chosen = p
		}
	}
	if chosen == nil {
		return "", "", coreerrors.New("CRM_PIPELINE_REQUIRED", "Buat pipeline CRM lebih dulu agar deal onboarding bisa dibuat.", http.StatusUnprocessableEntity)
	}
	stages := append([]crmdomain.PipelineStage(nil), chosen.Stages...)
	sort.SliceStable(stages, func(i, j int) bool { return stages[i].Position < stages[j].Position })
	for _, s := range stages {
		if !s.IsWon && !s.IsLost {
			return chosen.ID, s.ID, nil
		}
	}
	return "", "", coreerrors.New("CRM_PIPELINE_REQUIRED", "Pipeline CRM default belum punya tahap terbuka.", http.StatusUnprocessableEntity)
}

func (a mikrotikOnboardingCRM) CreateDeal(ctx context.Context, scope coretenant.Scope, d mikrotikservice.OnboardDeal, actor string) (string, error) {
	pipelineID, stageID, err := a.defaultStage(ctx, scope)
	if err != nil {
		return "", err
	}
	deal, err := a.deals.Create(ctx, scope, crmrepo.CreateDealParams{
		PipelineID: pipelineID, StageID: stageID, ContactID: d.ContactID, Title: d.Title, Value: d.Value,
		OwnerUserID: actor, CreatedBy: actor, Source: crmdomain.DealSourcePPPoEImport,
	})
	if err != nil {
		return "", err
	}
	return deal.ID, nil
}

func (a mikrotikOnboardingCRM) CreateQuotation(ctx context.Context, scope coretenant.Scope, q mikrotikservice.OnboardQuotation, actor string) (string, error) {
	created, err := a.quotations.Create(ctx, scope, crmservice.CreateQuotationInput{
		DealID: q.DealID, ContactID: q.ContactID, CreatedBy: actor,
		Items: []crmservice.QuotationLineInput{{ProductID: q.ProductID, Quantity: q.Quantity, UnitPrice: q.UnitPrice}},
	})
	if errors.Is(err, crmservice.ErrProductInactive) {
		return "", coreerrors.New("PRODUCT_INACTIVE", "Produk tidak aktif atau sudah dihapus.", http.StatusUnprocessableEntity)
	}
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// ApproveQuotation walks draft → sent → approved; approved is a no-op so a
// retry after a failure in a later step is safe.
func (a mikrotikOnboardingCRM) ApproveQuotation(ctx context.Context, scope coretenant.Scope, id, actor string) error {
	q, err := a.quotations.Get(ctx, scope, id)
	if err != nil {
		return err
	}
	switch q.Status {
	case crmdomain.QuotationStatusApproved:
		return nil
	case crmdomain.QuotationStatusDraft:
		if _, err := a.quotations.Send(ctx, scope, id, actor); err != nil {
			return err
		}
		fallthrough
	case crmdomain.QuotationStatusSent:
		_, err := a.quotations.Approve(ctx, scope, id, actor)
		return err
	}
	return onboardStepError("MIKROTIK_ONBOARDING_QUOTATION_STATE", "Penawaran onboarding berstatus "+string(q.Status)+" dan tidak bisa disetujui.")
}

// EnsureSalesOrder: the approve hook creates the SO but only logs its errors,
// so a missing SO is created here through the same idempotent hook.
func (a mikrotikOnboardingCRM) EnsureSalesOrder(ctx context.Context, scope coretenant.Scope, quotationID, actor string) (string, error) {
	so, err := a.soFinder.FindByQuotation(ctx, scope, quotationID)
	if err == nil {
		return so.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	q, err := a.quotations.Get(ctx, scope, quotationID)
	if err != nil {
		return "", err
	}
	if err := a.salesOrders.QuotationApproved(ctx, scope, q, actor); err != nil {
		return "", err
	}
	so, err = a.soFinder.FindByQuotation(ctx, scope, quotationID)
	if err != nil {
		return "", err
	}
	return so.ID, nil
}

// ConfirmSalesOrder fills and confirms a draft, or retries the billing of a
// confirmed one. The SO keeps its bill-to defaults (from the contact).
func (a mikrotikOnboardingCRM) ConfirmSalesOrder(ctx context.Context, scope coretenant.Scope, soID string, o mikrotikservice.ConfirmOrder, actor string) (mikrotikservice.ConfirmedOrder, error) {
	so, err := a.salesOrders.Get(ctx, scope, soID)
	if err != nil {
		return mikrotikservice.ConfirmedOrder{}, err
	}
	switch so.Status {
	case crmdomain.SalesOrderDraft:
		start := o.StartDate
		skip := o.SkipInitialInvoice
		if _, err := a.salesOrders.UpdateDraft(ctx, scope, soID, crmrepo.SalesOrderDraftFields{
			StartDate: &start, BillToName: so.BillToName, BillToCompany: so.BillToCompany, BillToEmail: so.BillToEmail,
			BillToPhone: so.BillToPhone, BillToAddress: so.BillToAddress, Channels: o.Channels, PICUserID: so.PICUserID,
			SkipInitialInvoice: &skip,
		}, actor); err != nil {
			return mikrotikservice.ConfirmedOrder{}, salesOrderError(err)
		}
		if so, err = a.salesOrders.Confirm(ctx, scope, soID, actor); err != nil {
			return mikrotikservice.ConfirmedOrder{}, salesOrderError(err)
		}
	case crmdomain.SalesOrderConfirmed, crmdomain.SalesOrderCompleted:
		if so.BillingStatus != crmdomain.BillingDone {
			if so, err = a.salesOrders.RetryBilling(ctx, scope, soID, actor); err != nil {
				return mikrotikservice.ConfirmedOrder{}, salesOrderError(err)
			}
		}
	default:
		return mikrotikservice.ConfirmedOrder{}, onboardStepError("MIKROTIK_ONBOARDING_SALES_ORDER_STATE", "Sales order onboarding sudah dibatalkan.")
	}
	if so.BillingStatus != crmdomain.BillingDone {
		msg := strings.TrimSpace(so.BillingError)
		if msg == "" {
			msg = "Penagihan sales order belum berhasil. Coba lagi."
		}
		return mikrotikservice.ConfirmedOrder{}, onboardStepError("MIKROTIK_ONBOARDING_BILLING_FAILED", msg)
	}
	if so.ContractID == "" {
		return mikrotikservice.ConfirmedOrder{}, onboardStepError("MIKROTIK_ONBOARDING_NO_CONTRACT", "Sales order tidak menghasilkan kontrak; pastikan produk ditagih berulang.")
	}
	return mikrotikservice.ConfirmedOrder{AccountID: so.ReceivableAccountID, ContractID: so.ContractID}, nil
}

func salesOrderError(err error) error {
	var inc *crmservice.IncompleteError
	switch {
	case errors.As(err, &inc):
		return coreerrors.New("SALES_ORDER_INCOMPLETE", "Data sales order belum lengkap: "+strings.Join(inc.Fields, ", ")+".", http.StatusUnprocessableEntity)
	case errors.Is(err, crmservice.ErrInvalidSalesOrderFields):
		return coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity)
	}
	return err
}

// ---- permissions ----

// mikrotikOnboardingPermissions: the onboarding acts on behalf of the
// operator, so every chain step's permission must be granted (organization
// role or global, like RequireOrganizationOrGlobal).
type mikrotikOnboardingPermissions struct {
	checker permissionmiddleware.CombinedPermissionChecker
}

var onboardingChainPermissions = []string{
	"deal.create", "quotation.create", "quotation.send", "quotation.approve", "sales_order.manage", "sales_order.confirm",
}

func (p mikrotikOnboardingPermissions) Missing(ctx context.Context, scope coretenant.Scope, actor string, newContact bool) ([]string, error) {
	if p.checker == nil {
		return nil, nil
	}
	needed := onboardingChainPermissions
	if newContact {
		needed = append([]string{"contact.create"}, needed...)
	}
	missing := []string{}
	for _, perm := range needed {
		if p.checker.CanOrganization(ctx, actor, scope.OrganizationID(), []string{perm}) == nil {
			continue
		}
		if p.checker.Can(ctx, actor, []string{perm}) == nil {
			continue
		}
		missing = append(missing, perm)
	}
	return missing, nil
}

// compile-time checks
var (
	_ mikrotikservice.OnboardingCRM         = mikrotikOnboardingCRM{}
	_ mikrotikservice.OnboardingPermissions = mikrotikOnboardingPermissions{}
)
