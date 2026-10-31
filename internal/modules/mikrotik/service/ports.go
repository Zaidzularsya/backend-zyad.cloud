package service

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
)

// AccountRef is the minimal customer account view the mikrotik module needs.
type AccountRef struct{ ID, Name, Phone, Email string }

// AccountDirectory reads customer accounts of the organization.
type AccountDirectory interface {
	// Get returns mikrotik.ErrAccountNotFound when the account is not owned by the organization.
	Get(ctx context.Context, scope coretenant.Scope, id string) (AccountRef, error)
	Search(ctx context.Context, scope coretenant.Scope, q string, limit int) ([]AccountRef, error)
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
