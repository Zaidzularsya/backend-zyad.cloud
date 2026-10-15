package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type companyService struct {
	repo       repository.CompanyRepository
	workspaces WorkspaceDirectory
	linkHook   WorkspaceLinkHook
}

// CompanyServiceOption mengaktifkan fitur tautan workspace (tanpa opsi, SetWorkspaceLink ditolak).
type CompanyServiceOption func(*companyService)

func WithWorkspaceLinking(workspaces WorkspaceDirectory, hook WorkspaceLinkHook) CompanyServiceOption {
	return func(s *companyService) { s.workspaces, s.linkHook = workspaces, hook }
}

func NewCompanyService(repo repository.CompanyRepository, opts ...CompanyServiceOption) CompanyService {
	s := &companyService{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *companyService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreateCompanyParams) (domain.Company, error) {
	return s.repo.Create(ctx, scope, params)
}

func (s *companyService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Company, error) {
	company, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Company{}, crmmodule.MapNotFound(err, "COMPANY_NOT_FOUND", "company not found or already deleted")
	}
	return company, nil
}

func (s *companyService) List(ctx context.Context, scope coretenant.Scope, filter repository.CompanyListFilter) ([]domain.Company, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *companyService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateCompanyParams) (domain.Company, error) {
	company, err := s.repo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.Company{}, crmmodule.MapNotFound(err, "COMPANY_NOT_FOUND", "company not found or already deleted")
	}
	return company, nil
}

func (s *companyService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "COMPANY_NOT_FOUND", "company not found or already deleted")
}

func (s *companyService) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	return crmmodule.MapNotFound(s.repo.Restore(ctx, scope, id, restoredBy), "COMPANY_NOT_FOUND", "company not found or not deleted")
}

// FindSimilar mencari company yang namanya mirip (maks 5). Kandidat diambil
// dengan kata terpanjang dari nama ter-normalisasi, lalu disaring di Go.
func (s *companyService) FindSimilar(ctx context.Context, scope coretenant.Scope, name string) ([]domain.Company, error) {
	normalized := NormalizeCompanyName(name)
	if normalized == "" {
		return []domain.Company{}, nil
	}
	longest := ""
	for _, w := range strings.Fields(normalized) {
		if len([]rune(w)) > len([]rune(longest)) {
			longest = w
		}
	}
	candidates, err := s.repo.FindCandidatesByName(ctx, scope, longest, 25)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Company, 0, 5)
	for _, c := range candidates {
		if isSimilarCompanyName(normalized, c.Name) {
			out = append(out, c)
			if len(out) == 5 {
				break
			}
		}
	}
	return out, nil
}

// ErrWorkspaceNotFound dikembalikan WorkspaceDirectory bila workspace tidak ada.
var ErrWorkspaceNotFound = errors.New("workspace not found")

func (s *companyService) Workspace(ctx context.Context, tenantOrgID string) *WorkspaceInfo {
	if s.workspaces == nil || tenantOrgID == "" {
		return nil
	}
	info, err := s.workspaces.FindWorkspace(ctx, tenantOrgID)
	if err != nil {
		slog.WarnContext(ctx, "company workspace lookup failed", "tenant_organization_id", tenantOrgID, "error", err)
		return nil
	}
	return &info
}

func (s *companyService) SetWorkspaceLink(ctx context.Context, scope coretenant.Scope, companyID string, tenantOrgID *string, actorUserID string) (domain.Company, error) {
	if s.workspaces == nil {
		return domain.Company{}, coreerrors.New("WORKSPACE_LINK_UNAVAILABLE", "workspace linking is not configured", http.StatusServiceUnavailable)
	}
	current, err := s.repo.FindByID(ctx, scope, companyID)
	if err != nil {
		return domain.Company{}, crmmodule.MapNotFound(err, "COMPANY_NOT_FOUND", "company not found or already deleted")
	}
	target := ""
	if tenantOrgID != nil {
		target = strings.TrimSpace(*tenantOrgID)
	}
	if target != "" {
		ws, err := s.workspaces.FindWorkspace(ctx, target)
		if errors.Is(err, ErrWorkspaceNotFound) {
			return domain.Company{}, coreerrors.New("WORKSPACE_NOT_FOUND", "workspace not found", http.StatusUnprocessableEntity)
		}
		if err != nil {
			return domain.Company{}, err
		}
		if !ws.Customer {
			return domain.Company{}, coreerrors.New("WORKSPACE_NOT_CUSTOMER", "only customer workspaces can be linked to a company", http.StatusUnprocessableEntity)
		}
	}
	previous := ""
	if current.TenantOrganizationID != nil {
		previous = *current.TenantOrganizationID
	}
	if previous == target {
		return current, nil
	}
	updated, err := s.repo.SetTenantOrganization(ctx, scope, companyID, target, actorUserID)
	if errors.Is(err, repository.ErrWorkspaceAlreadyLinked) {
		return domain.Company{}, coreerrors.New("WORKSPACE_ALREADY_LINKED", "workspace is already linked to another company", http.StatusConflict)
	}
	if err != nil {
		return domain.Company{}, crmmodule.MapNotFound(err, "COMPANY_NOT_FOUND", "company not found or already deleted")
	}
	if s.linkHook != nil {
		// Setelah commit: galat hanya dicatat; SyncContract aman diulang dari event invoice/contract berikutnya.
		if err := s.linkHook.OnWorkspaceLinkChanged(ctx, scope, companyID, nilIfEmpty(previous), nilIfEmpty(target)); err != nil {
			slog.WarnContext(ctx, "workspace link sync failed", "company_id", companyID, "error", err)
		}
	}
	return updated, nil
}

func nilIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
