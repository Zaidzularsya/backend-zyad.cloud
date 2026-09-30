package repository

import (
	"context"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type LeadListFilter struct {
	Search      string
	Status      domain.LeadStatus
	OwnerUserID string
	// ConvertedContactID: lead yang di-convert menjadi contact ini.
	ConvertedContactID string
	// Source matches case-insensitively.
	Source string
	// CreatedFrom (inclusive) / CreatedToExclusive bound created_at; zero
	// value = unbounded.
	CreatedFrom        time.Time
	CreatedToExclusive time.Time
	// Sort is one of LeadSortFields, optionally prefixed with "-" for
	// descending. Empty = "-created_at".
	Sort           string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// LeadSortFields maps the public sort keys to SQL expressions. Status sorts
// in pipeline order rather than alphabetically.
var LeadSortFields = map[string]string{
	"created_at":   "created_at",
	"updated_at":   "updated_at",
	"contact_name": "lower(contact_name)",
	"score":        "score",
	"status":       "CASE status WHEN 'new' THEN 1 WHEN 'attempting' THEN 2 WHEN 'contacted' THEN 3 WHEN 'qualified' THEN 4 WHEN 'unqualified' THEN 5 ELSE 6 END",
}

// IsValidLeadSort reports whether sort is empty or a known (optionally
// "-"-prefixed) sort key.
func IsValidLeadSort(sort string) bool {
	if sort == "" {
		return true
	}
	_, ok := LeadSortFields[strings.TrimPrefix(sort, "-")]
	return ok
}

func leadOrderBy(sort string) string {
	if sort == "" {
		sort = "-created_at"
	}
	direction := "ASC"
	if strings.HasPrefix(sort, "-") {
		direction = "DESC"
		sort = strings.TrimPrefix(sort, "-")
	}
	expr, ok := LeadSortFields[sort]
	if !ok {
		expr, direction = "created_at", "DESC"
	}
	// id keeps pagination stable when the sort key has ties.
	return expr + " " + direction + ", id " + direction
}

// CreateLeadParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope.
type CreateLeadParams struct {
	ContactName   string
	CompanyName   string
	Email         string
	Phone         string
	Source        string
	Score         int
	OwnerUserID   string
	Notes         string
	JobTitle      string
	AnnualRevenue string
	Address       map[string]any
	CreatedBy     string
}

type UpdateLeadParams struct {
	ContactName   *string
	CompanyName   *string
	Email         *string
	Phone         *string
	Source        *string
	Status        *domain.LeadStatus
	Score         *int
	OwnerUserID   *string
	Notes         *string
	JobTitle      *string
	AnnualRevenue *string
	// Address nil = tidak diubah; map kosong = kosongkan.
	Address   map[string]any
	UpdatedBy string
}

// MarkConvertedParams records the result of converting a lead into working
// CRM records. Called after the Contact/Company (and, from Fase 2 onward,
// Deal) rows already exist.
type MarkConvertedParams struct {
	ConvertedContactID string
	ConvertedCompanyID string
	UpdatedBy          string
}

// LeadRepository is the tenant-owned data contract. Every method requires a
// verified immutable scope and row lookups include both scope and resource ID.
type LeadRepository interface {
	Create(context.Context, coretenant.Scope, CreateLeadParams) (domain.Lead, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Lead, error)
	List(context.Context, coretenant.Scope, LeadListFilter) ([]domain.Lead, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateLeadParams) (domain.Lead, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
	Assign(context.Context, coretenant.Scope, string, string, string) (domain.Lead, error)
	MarkConverted(context.Context, coretenant.Scope, string, MarkConvertedParams) (domain.Lead, error)
	// FindActiveByPhone returns the most recently updated non-deleted,
	// non-converted lead whose phone_normalized matches (WhatsApp matching),
	// or pgx.ErrNoRows.
	FindActiveByPhone(ctx context.Context, scope coretenant.Scope, phoneNormalized string) (domain.Lead, error)
}

// LeadDashboardRepository reads the aggregates behind the Leads overview
// page. Read-only; every query is scoped to the organization.
type LeadDashboardRepository interface {
	Dashboard(context.Context, coretenant.Scope, domain.LeadDashboardRange, LeadDashboardLimits) (domain.LeadDashboard, error)
}

type LeadDashboardLimits struct {
	FollowUps      int
	RecentActivity int
}
