//go:build integration

package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestDomainServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	pageRepo := repository.NewPageRepository(db)
	domainRepo := repository.NewDomainRepository(db)
	domainSvc := service.NewDomainService(domainRepo, pageRepo, db)

	scope := tenants.A.Scope

	// Insert test organization domain
	organizationDomainID := "55555555-5555-5555-5555-555555555555"
	_, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-domain-test', 'Organization Domain Test', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID)
	require.NoError(t, err)

	_, err = db.Exec(ctx, `
		INSERT INTO organization_domains (id, organization_id, type, canonical_host, status, ssl_status, verified_at)
		VALUES ($1, $2, 'custom', 'test.domain.com', 'verified', 'active', now())
		ON CONFLICT DO NOTHING
	`, organizationDomainID, tenants.A.OrganizationID)
	require.NoError(t, err)

	// Insert dummy user
	_, err = db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES ('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	require.NoError(t, err)

	// Create test page
	page, err := pageRepo.Create(ctx, scope, repository.CreatePageParams{
		Name:       "Domain Test Page",
		Title:      "Domain Test Page",
		Slug:       fmt.Sprintf("domain-test-page-%d", time.Now().UnixNano()),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	require.NoError(t, err)

	t.Run("ListAvailableDomains", func(t *testing.T) {
		domains, err := domainSvc.ListAvailableDomains(ctx, scope)
		require.NoError(t, err)
		assert.NotEmpty(t, domains)
		
		found := false
		for _, d := range domains {
			if d.ID == organizationDomainID {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected to find test organization domain")
	})

	t.Run("BindDomain_and_ListBindings", func(t *testing.T) {
		binding, err := domainSvc.BindDomain(ctx, scope, service.BindDomainParams{
			OrganizationDomainID: organizationDomainID,
			LandingPageID:        page.ID,
			IsPrimary:            false, // Service should force this to true since it's the first binding
		})
		require.NoError(t, err)
		assert.NotEmpty(t, binding.ID)
		assert.True(t, binding.IsPrimary)

		bindings, err := domainSvc.ListBindings(ctx, scope, page.ID)
		require.NoError(t, err)
		require.Len(t, bindings, 1)
		assert.Equal(t, binding.ID, bindings[0].ID)

		// Try to bind again (should fail)
		_, err = domainSvc.BindDomain(ctx, scope, service.BindDomainParams{
			OrganizationDomainID: organizationDomainID,
			LandingPageID:        page.ID,
			IsPrimary:            false,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already bound")
	})

	t.Run("SetPrimaryBinding", func(t *testing.T) {
		// Create another domain to bind
		orgDomainID2 := "66666666-6666-6666-6666-666666666666"
		_, err := db.Exec(ctx, `
			INSERT INTO organization_domains (id, organization_id, type, canonical_host, status, ssl_status, verified_at)
			VALUES ($1, $2, 'custom', 'test2.domain.com', 'verified', 'active', now())
			ON CONFLICT DO NOTHING
		`, orgDomainID2, tenants.A.OrganizationID)
		require.NoError(t, err)

		binding2, err := domainSvc.BindDomain(ctx, scope, service.BindDomainParams{
			OrganizationDomainID: orgDomainID2,
			LandingPageID:        page.ID,
			IsPrimary:            false,
		})
		require.NoError(t, err)
		assert.False(t, binding2.IsPrimary)

		// Set the second binding as primary
		err = domainSvc.SetPrimaryBinding(ctx, scope, page.ID, binding2.ID)
		require.NoError(t, err)

		bindings, err := domainSvc.ListBindings(ctx, scope, page.ID)
		require.NoError(t, err)
		require.Len(t, bindings, 2)

		for _, b := range bindings {
			if b.ID == binding2.ID {
				assert.True(t, b.IsPrimary)
			} else {
				assert.False(t, b.IsPrimary)
			}
		}
	})

	t.Run("UnbindDomain", func(t *testing.T) {
		bindings, err := domainSvc.ListBindings(ctx, scope, page.ID)
		require.NoError(t, err)
		require.NotEmpty(t, bindings)

		err = domainSvc.UnbindDomain(ctx, scope, bindings[0].ID)
		require.NoError(t, err)

		newBindings, err := domainSvc.ListBindings(ctx, scope, page.ID)
		require.NoError(t, err)
		assert.Len(t, newBindings, len(bindings)-1)
	})
}
