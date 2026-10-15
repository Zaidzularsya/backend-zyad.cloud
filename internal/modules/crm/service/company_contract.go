package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type CompanyService interface {
	Create(context.Context, coretenant.Scope, repository.CreateCompanyParams) (domain.Company, error)
	FindSimilar(ctx context.Context, scope coretenant.Scope, name string) ([]domain.Company, error)
	Get(context.Context, coretenant.Scope, string) (domain.Company, error)
	List(context.Context, coretenant.Scope, repository.CompanyListFilter) ([]domain.Company, int64, error)
	Update(context.Context, coretenant.Scope, string, repository.UpdateCompanyParams) (domain.Company, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
	// SetWorkspaceLink menautkan company ke workspace pelanggan (tenantOrgID nil = melepas) lalu menyinkronkan
	// akses workspace lama dan baru. Hanya org platform yang boleh memanggilnya; izin diperiksa di handler.
	SetWorkspaceLink(ctx context.Context, scope coretenant.Scope, companyID string, tenantOrgID *string, actorUserID string) (domain.Company, error)
	// Workspace meringkas workspace tertaut untuk respons; nil bila tidak ada.
	Workspace(ctx context.Context, tenantOrgID string) *WorkspaceInfo
}

// WorkspaceInfo adalah ringkasan workspace pelanggan; diadaptasi dari modul organization di internal/app.
type WorkspaceInfo struct {
	ID, Name, Slug, Status string
	Customer               bool // type == customer
}

// WorkspaceDirectory membaca workspace; ErrWorkspaceNotFound bila tidak ada.
type WorkspaceDirectory interface {
	FindWorkspace(ctx context.Context, id string) (WorkspaceInfo, error)
}

// WorkspaceLinkHook dipanggil setelah tautan company berubah (old/new nil = tidak ada).
type WorkspaceLinkHook interface {
	OnWorkspaceLinkChanged(ctx context.Context, scope coretenant.Scope, companyID string, oldTenantOrgID, newTenantOrgID *string) error
}
