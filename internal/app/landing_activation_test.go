package app

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	landingrepo "zyad.cloud/internal/modules/landing/repository"
	landingservice "zyad.cloud/internal/modules/landing/service"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

const activationOrgID = "11111111-1111-1111-1111-111111111111"

func activationScope(t *testing.T) coretenant.Scope {
	t.Helper()
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: activationOrgID, OrganizationSlug: "reseller",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID: "22222222-2222-2222-2222-222222222222", MembershipStatus: "active", MembershipVersion: 1,
		ResolutionSource: coretenant.ResolutionSourceSession, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

type fakeActivationAccounts struct {
	receivableservice.AccountService
	got receivableservice.AccountInput
	err error
}

func (f *fakeActivationAccounts) Create(_ context.Context, _ coretenant.Scope, in receivableservice.AccountInput, _ string) (receivabledomain.Account, error) {
	f.got = in
	return receivabledomain.Account{ID: "acc-1"}, f.err
}

type fakeActivationInvoices struct {
	receivableservice.InvoiceService
	got               receivableservice.InvoiceInput
	orgs              []string
	issueErr, linkErr error
	voided            []string
}

func (f *fakeActivationInvoices) CreateAndIssue(_ context.Context, scope coretenant.Scope, in receivableservice.InvoiceInput, _ string) (receivabledomain.Invoice, error) {
	f.got = in
	f.orgs = append(f.orgs, scope.OrganizationID())
	return receivabledomain.Invoice{ID: "inv-1"}, f.issueErr
}

func (f *fakeActivationInvoices) Link(context.Context, coretenant.Scope, string, string) (string, time.Time, error) {
	return "https://app.test/i/abc", time.Time{}, f.linkErr
}

func (f *fakeActivationInvoices) Void(_ context.Context, _ coretenant.Scope, id, _, _ string) (receivabledomain.Invoice, error) {
	f.voided = append(f.voided, id)
	return receivabledomain.Invoice{}, nil
}

func activationRequest() landingservice.ActivationInvoiceRequest {
	return landingservice.ActivationInvoiceRequest{
		ActivationID: "act-1", BuyerName: "Budi", BuyerContact: "0812", BuyerEmail: "b@example.test",
		Description: "Halaman undangan budi", Price: "150000.00", DueAt: time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC),
	}
}

func TestActivationBillingIssuesManualInvoiceInScope(t *testing.T) {
	accounts, invoices := &fakeActivationAccounts{}, &fakeActivationInvoices{}
	b := receivableActivationBilling{accounts: accounts, invoices: invoices}
	res, err := b.IssueInvoice(context.Background(), activationScope(t), activationRequest(), "user-1")
	if err != nil || res.InvoiceID != "inv-1" || res.PaymentURL != "https://app.test/i/abc" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if accounts.got.Name != "Budi" || accounts.got.Phone != "0812" || accounts.got.Email != "b@example.test" || accounts.got.SourceType != "" {
		t.Fatalf("account input = %+v", accounts.got)
	}
	in := invoices.got
	if in.SourceType != receivabledomain.SourceManual || in.SourceID != "act-1" || in.IdempotencyKey != "act-1" ||
		in.AccountID != "acc-1" || in.DueDate == nil || !in.DueDate.Equal(activationRequest().DueAt) || len(in.Channels) != 0 {
		t.Fatalf("invoice input = %+v", in)
	}
	if len(in.Lines) != 1 || in.Lines[0].Description != "Halaman undangan budi" || in.Lines[0].UnitPrice != "150000.00" || in.Lines[0].Quantity != "1" {
		t.Fatalf("lines = %+v", in.Lines)
	}
	if len(invoices.orgs) != 1 || invoices.orgs[0] != activationOrgID {
		t.Fatalf("invoice must be created in the reseller org, got %v", invoices.orgs)
	}
}

func TestActivationBillingErrors(t *testing.T) {
	// Link failure: invoice id is returned so the caller can void it.
	b := receivableActivationBilling{accounts: &fakeActivationAccounts{}, invoices: &fakeActivationInvoices{linkErr: errors.New("db")}}
	res, err := b.IssueInvoice(context.Background(), activationScope(t), activationRequest(), "u")
	var appErr *coreerrors.AppError
	if res.InvoiceID != "inv-1" || !errors.As(err, &appErr) || appErr.Code != "ACTIVATION_INVOICE_FAILED" {
		t.Fatalf("link failure: res=%+v err=%v", res, err)
	}
	// Quota AppError passes through unchanged.
	quota := coreerrors.New("QUOTA_EXCEEDED", "q", http.StatusConflict)
	b = receivableActivationBilling{accounts: &fakeActivationAccounts{}, invoices: &fakeActivationInvoices{issueErr: quota}}
	if _, err := b.IssueInvoice(context.Background(), activationScope(t), activationRequest(), "u"); !errors.Is(err, quota) {
		t.Fatalf("quota: err=%v", err)
	}
	// Receivable validation error becomes 422.
	b = receivableActivationBilling{accounts: &fakeActivationAccounts{err: receivableservice.ErrInvalidAccount}, invoices: &fakeActivationInvoices{}}
	_, err = b.IssueInvoice(context.Background(), activationScope(t), activationRequest(), "u")
	if !errors.As(err, &appErr) || appErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("validation: err=%v", err)
	}
}

type fakeActivationRepoForChecker struct {
	landingrepo.ActivationRepository
	exists bool
	err    error
	orgs   []string
}

func (f *fakeActivationRepoForChecker) ExistsForInvoice(_ context.Context, scope coretenant.Scope, _ string) (bool, error) {
	f.orgs = append(f.orgs, scope.OrganizationID())
	return f.exists, f.err
}

func TestActivationInvoiceChecker(t *testing.T) {
	repo := &fakeActivationRepoForChecker{exists: true}
	ok, err := activationInvoiceChecker{activations: repo}.IsActivationInvoice(context.Background(), activationScope(t), "inv-1")
	if !ok || err != nil || repo.orgs[0] != activationOrgID {
		t.Fatalf("ok=%v err=%v orgs=%v", ok, err, repo.orgs)
	}
	if _, err := (activationInvoiceChecker{}).IsActivationInvoice(context.Background(), activationScope(t), "inv-1"); err == nil {
		t.Fatal("unconfigured checker must fail closed")
	}
}

type fakeDomainLister struct {
	domains []organizationmodel.OrganizationDomain
	err     error
	filter  organizationrepo.DomainListFilter
	org     string
}

func (f *fakeDomainLister) ListByOrganization(_ context.Context, orgID string, filter organizationrepo.DomainListFilter) ([]organizationmodel.OrganizationDomain, int64, error) {
	f.org, f.filter = orgID, filter
	return f.domains, int64(len(f.domains)), f.err
}

func TestActivationPreviewLinks(t *testing.T) {
	lister := &fakeDomainLister{domains: []organizationmodel.OrganizationDomain{
		{OrganizationID: activationOrgID, CanonicalHost: "reseller.zyad.cloud", IsPrimary: true},
		{OrganizationID: activationOrgID, CanonicalHost: "other.example.test"},
	}}
	url, err := activationPreviewLinks{domains: lister}.PreviewURL(context.Background(), activationScope(t), "TOKEN")
	if err != nil || url != "https://reseller.zyad.cloud/api/v1/public/landing/preview/TOKEN" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	if lister.org != activationOrgID || lister.filter.Status != organizationmodel.DomainStatusActive {
		t.Fatalf("lookup org=%s filter=%+v", lister.org, lister.filter)
	}

	empty := &fakeDomainLister{}
	if url, _ := (activationPreviewLinks{domains: empty}).PreviewURL(context.Background(), activationScope(t), "TOKEN"); url != "/api/v1/public/landing/preview/TOKEN" {
		t.Fatalf("fallback url=%q", url)
	}
	if _, err := (activationPreviewLinks{domains: &fakeDomainLister{err: errors.New("db")}}).PreviewURL(context.Background(), activationScope(t), "T"); err == nil {
		t.Fatal("lookup error must be returned")
	}
}
