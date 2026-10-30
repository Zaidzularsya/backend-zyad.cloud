package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type fakeOwners struct {
	owners []domain.OrganizationMember
	err    error
}

func (f fakeOwners) ListOwners(context.Context, string) ([]domain.OrganizationMember, error) {
	return f.owners, f.err
}

// fakeTenantPipelines mengimplementasikan repository.PipelineRepository penuh agar tes bisa membuktikan
// Create/ReplaceStages tidak pernah dipanggil.
type fakeTenantPipelines struct {
	repository.PipelineRepository
	system  *domain.Pipeline
	list    []domain.Pipeline
	created int
}

func (f *fakeTenantPipelines) FindBySystemKey(context.Context, coretenant.Scope, string) (domain.Pipeline, error) {
	if f.system == nil {
		return domain.Pipeline{}, pgx.ErrNoRows
	}
	return *f.system, nil
}
func (f *fakeTenantPipelines) List(context.Context, coretenant.Scope, repository.PipelineListFilter) ([]domain.Pipeline, int64, error) {
	return f.list, int64(len(f.list)), nil
}
func (f *fakeTenantPipelines) Create(context.Context, coretenant.Scope, repository.CreatePipelineParams) (domain.Pipeline, error) {
	f.created++
	return domain.Pipeline{}, errors.New("must not be called")
}

var tenantOwner = domain.OrganizationMember{UserID: "owner-1", Email: "o@t.test"}

func TestSelfServeResolver_Tenant_UsesOwnerAndDefaultPipeline(t *testing.T) {
	now := time.Now()
	p := &fakeTenantPipelines{list: []domain.Pipeline{
		{ID: "newer", CreatedAt: now},
		{ID: "archived-old", CreatedAt: now.Add(-3 * time.Hour), ArchivedAt: &now},
		{ID: "oldest", CreatedAt: now.Add(-2 * time.Hour)},
	}}
	cfg, err := NewTenantSelfServeResolver(fakeOwners{owners: []domain.OrganizationMember{tenantOwner}}, p).Resolve(context.Background(), coretenant.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg != (SelfServeConfig{BotUserID: "owner-1", DealOwnerUserID: "owner-1", PipelineID: "oldest", TenantScope: true}) {
		t.Fatalf("cfg = %+v", cfg)
	}
	// pipeline sistem "self_serve" menang atas default.
	p.system = &domain.Pipeline{ID: "sys"}
	cfg, _ = NewTenantSelfServeResolver(fakeOwners{owners: []domain.OrganizationMember{tenantOwner}}, p).Resolve(context.Background(), coretenant.Scope{})
	if cfg.PipelineID != "sys" {
		t.Fatalf("pipeline = %s, want sys", cfg.PipelineID)
	}
}

func TestSelfServeResolver_Tenant_NoPipeline_NotConfigured(t *testing.T) {
	p := &fakeTenantPipelines{}
	_, err := NewTenantSelfServeResolver(fakeOwners{owners: []domain.OrganizationMember{tenantOwner}}, p).Resolve(context.Background(), coretenant.Scope{})
	if !errors.Is(err, ErrSelfServeNotConfigured) {
		t.Fatalf("err = %v, want ErrSelfServeNotConfigured", err)
	}
	_, err = NewTenantSelfServeResolver(fakeOwners{}, &fakeTenantPipelines{list: []domain.Pipeline{{ID: "x"}}}).Resolve(context.Background(), coretenant.Scope{})
	if !errors.Is(err, ErrSelfServeNotConfigured) {
		t.Fatalf("no owner: err = %v, want ErrSelfServeNotConfigured", err)
	}
}

func TestSelfServeResolver_Tenant_NeverCreatesPipeline(t *testing.T) {
	p := &fakeTenantPipelines{}
	r := NewTenantSelfServeResolver(fakeOwners{owners: []domain.OrganizationMember{tenantOwner}}, p)
	_, _ = r.Resolve(context.Background(), coretenant.Scope{})
	if p.created != 0 {
		t.Fatalf("created = %d, want 0", p.created)
	}
}

type fakePlatformScope struct{ scope coretenant.Scope }

func (f fakePlatformScope) PlatformScope(context.Context) (coretenant.Scope, error) {
	return f.scope, nil
}

type markerResolver string

func (m markerResolver) Resolve(context.Context, coretenant.Scope) (SelfServeConfig, error) {
	return SelfServeConfig{PipelineID: string(m)}, nil
}

func TestSelfServeResolver_RoutesByOrganization(t *testing.T) {
	platform := validTestScope(t, "org-platform")
	r := NewScopedSelfServeResolver(fakePlatformScope{platform}, markerResolver("platform"), markerResolver("tenant"))
	for scope, want := range map[coretenant.Scope]string{platform: "platform", validTestScope(t, "org-t1"): "tenant"} {
		cfg, err := r.Resolve(context.Background(), scope)
		if err != nil || cfg.PipelineID != want {
			t.Fatalf("org %s: cfg=%+v err=%v want %s", scope.OrganizationID(), cfg, err, want)
		}
	}
}
