package service

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

// AccountRef is the minimal customer account view the mikrotik module needs.
type AccountRef struct{ ID, Name, Phone, Email string }

// AccountDirectory reads customer accounts of the organization.
type AccountDirectory interface {
	// Get returns mikrotik.ErrAccountNotFound when the account is not owned by the organization.
	Get(ctx context.Context, scope coretenant.Scope, id string) (AccountRef, error)
	Search(ctx context.Context, scope coretenant.Scope, q string, limit int) ([]AccountRef, error)
	// List pages through all accounts ordered by id ascending (keyset; afterID "" = from the start).
	List(ctx context.Context, scope coretenant.Scope, afterID string, limit int) ([]AccountRef, error)
}

// ContactProvisioner creates a CRM contact and account from a PPP secret.
type ContactProvisioner interface {
	CreateCustomerFromSecret(ctx context.Context, scope coretenant.Scope, name, comment, actorUserID string) (AccountRef, error)
}

// ProductInfo is the minimal product view the mikrotik module needs.
type ProductInfo struct {
	ID, Name, SKU     string
	Recurring, Active bool
}

// ProductCatalog reads products of the organization.
type ProductCatalog interface {
	// Get returns mikrotik.ErrProductNotFound when the product is not owned by the organization.
	Get(ctx context.Context, scope coretenant.Scope, id string) (ProductInfo, error)
	// ListRecurringInUse lists recurring products used by active contracts.
	ListRecurringInUse(ctx context.Context, scope coretenant.Scope) ([]ProductInfo, error)
}

// ProductUsage counts what a product trigger would affect.
type ProductUsage struct{ ActiveContracts, Accounts, OverdueInvoices int }

// UsageReader measures the impact of enabling a product trigger.
type UsageReader interface {
	ForProduct(ctx context.Context, scope coretenant.Scope, productID string, effectiveFrom time.Time) (ProductUsage, error)
}

// TriggerInvoice is an unpaid, past-due invoice that counts toward isolir.
// Balance is a decimal string (grand_total - amount_paid).
type TriggerInvoice struct {
	ID, Number string
	DueDate    time.Time
	Balance    string
}

// OverdueSource reads the receivable data the isolation evaluator needs.
type OverdueSource interface {
	// TriggerInvoices: candidate invoices of the account (Global Constraints
	// rules), ordered by due_date. today is a business-day date (UTC midnight);
	// an invoice due today is not a candidate.
	TriggerInvoices(ctx context.Context, scope coretenant.Scope, accountID string, today time.Time) ([]TriggerInvoice, error)
	// AccountsWithTriggerInvoices: accounts with >=1 candidate, ordered by
	// account id, keyset paging (afterAccountID "" = from the start).
	AccountsWithTriggerInvoices(ctx context.Context, scope coretenant.Scope, today time.Time, afterAccountID string, limit int) ([]string, error)
	// AccountOfInvoice returns the invoice's account_id;
	// mikrotik.ErrAccountNotFound when the invoice is not owned by the organization.
	AccountOfInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) (string, error)
}

// RouterReader loads one router of the organization; satisfied by *RouterService.
type RouterReader interface {
	// Get returns mikrotik.ErrRouterNotFound when absent (or deleted).
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Router, error)
}

// RouterLocker serialises router work across workers and instances; satisfied
// by *repository.AdvisoryLocker. ok=false means another holder has the key.
type RouterLocker interface {
	TryLock(ctx context.Context, key string) (func(), bool, error)
}

// CustomerNotifier tells the customer about an isolate/restore. Failures are
// only logged by the caller: they never undo or fail the router change.
type CustomerNotifier interface {
	NotifyIsolated(ctx context.Context, scope coretenant.Scope, accountID string, invoices []TriggerInvoice) error
	NotifyRestored(ctx context.Context, scope coretenant.Scope, accountID string) error
}

// TenantAlerter tells the tenant that an isolation task keeps failing.
type TenantAlerter interface {
	NotifyTaskFailed(ctx context.Context, scope coretenant.Scope, e TaskFailureEvent) error
}

// TaskFailureEvent is the tenant-facing description of a failing task. Reason
// is TaskFailure.Message (fixed, credential-free text).
type TaskFailureEvent struct{ TaskID, RouterName, SecretName, Action, Reason string }

// SenderSettings tells whether the organization has a default sender
// configured for customer notifications (owned by the receivable module).
type SenderSettings interface {
	HasDefaultSender(ctx context.Context, scope coretenant.Scope) (bool, error)
}

// Candidate statuses of the isolation preview, in priority order (the first
// matching one wins; see CandidateStatusOf).
const (
	CandidateExempt          = "exempt"
	CandidateNoSecret        = "no_secret"
	CandidateAlreadyIsolated = "already_isolated"
	CandidateNeedsProfile    = "needs_profile"
	CandidateSecretMissing   = "secret_missing"
	CandidateRouterNotReady  = "router_not_ready"
	CandidateWillIsolate     = "will_isolate"
)

// CandidateFilter narrows the candidate list. Status is one of the
// Candidate* statuses ("" = all); Q is a case-insensitive substring of the
// account name or phone; AccountIDs restricts to those accounts (nil = all).
// After switches to keyset paging: rows strictly after the cursor in
// (oldest_due, account_id) order; offset is then ignored and the total is not
// counted (0). StatusOnly skips the per-row product names.
type CandidateFilter struct {
	Status, Q  string
	AccountIDs []string
	After      *CandidateCursor
	StatusOnly bool
}

// CandidateCursor is the sort key of the last row of a page.
type CandidateCursor struct {
	OldestDue time.Time
	AccountID string
}

// CandidateRow is one account with at least one candidate invoice.
// OverdueDays is counted from the oldest candidate due date to today;
// Balance is a decimal string (sum of grand_total - amount_paid); Products
// lists the trigger product names of the candidate invoices (", "-joined);
// RouterName is the router of the account's first paired secret.
type CandidateRow struct {
	AccountID, AccountName, Phone, Status, RouterName string
	OldestDue                                         time.Time
	Products                                          string
	OverdueDays, InvoiceCount                         int
	OldestInvoiceNumber                               string
	Balance                                           string
	ExemptUntil                                       *time.Time
	Pilot                                             bool
}

// CandidateSummary counts the preview. Blocked = no_secret + needs_profile +
// secret_missing + router_not_ready; PairedAccounts are all accounts of the
// organization with a paired secret on a non-deleted router; Safe =
// PairedAccounts - Target.
type CandidateSummary struct {
	Target, Exempt, Blocked, AlreadyIsolated, PairedAccounts, Safe int
	GraceDays                                                      int
	Cutoff                                                         time.Time
}

// CandidateSource is the isolation candidate read model. today is the WIB
// business day (UTC midnight) used for exemptions and overdue days; cutoff is
// CandidateCutoff(today, graceDays) and decides which invoices are candidates.
type CandidateSource interface {
	// ListCandidates is ordered by oldest candidate due date, then account id;
	// total counts every row matching f (except on keyset pages, see After).
	ListCandidates(ctx context.Context, scope coretenant.Scope, today, cutoff time.Time, f CandidateFilter, limit, offset int) ([]CandidateRow, int64, error)
	// Summary fills every count except GraceDays, Cutoff and Safe.
	Summary(ctx context.Context, scope coretenant.Scope, today, cutoff time.Time) (CandidateSummary, error)
}

// PublicDirectory finds customer accounts for the public isolir page. Matching
// (case-insensitive, trimmed) is the implementation's job; the service passes
// the trimmed query.
type PublicDirectory interface {
	// AccountsByUsername returns accounts having a secret named username in the scope's organization.
	AccountsByUsername(ctx context.Context, scope coretenant.Scope, username string) ([]AccountRef, error)
	// AccountsByPhone returns accounts whose normalized phone equals normalizedPhone.
	AccountsByPhone(ctx context.Context, scope coretenant.Scope, normalizedPhone string) ([]AccountRef, error)
}

// PublicPayLinks builds the payment link of an invoice.
type PublicPayLinks interface {
	// PayLink returns "" without error when the tenant has no default sender.
	PayLink(ctx context.Context, scope coretenant.Scope, invoiceID string) (string, error)
}

// TenantContact is the tenant contact shown on the public page.
type TenantContact struct{ Name, Phone, Email string }

// PublicTenantProfile reads the tenant contact.
type PublicTenantProfile interface {
	Profile(ctx context.Context, scope coretenant.Scope) (TenantContact, error)
}

// PublicFeatureGate tells whether the public isolir page is enabled for the tenant.
type PublicFeatureGate interface {
	Enabled(ctx context.Context, scope coretenant.Scope) (bool, error)
}

// PublicSecrets is the SecretRepository subset the public page needs.
type PublicSecrets interface {
	ListByAccount(ctx context.Context, scope coretenant.Scope, accountID string) ([]domain.PPPSecret, error)
}
