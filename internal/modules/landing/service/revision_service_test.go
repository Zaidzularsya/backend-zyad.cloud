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

func TestRevisionServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	scope := tenants.A.Scope

	pageRepo := repository.NewPageRepository(db)
	revisionRepo := repository.NewRevisionRepository(db)
	revisionSvc := service.NewRevisionService(revisionRepo, pageRepo)

	// Insert organization for foreign key constraint
	_, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-rev', 'Organization Rev', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID)
	require.NoError(t, err)

	_, err = db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES 
		('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active'),
		('22222222-2222-2222-2222-222222222222', 'Mock User B', 'mock_b@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	require.NoError(t, err)

	// Create test page
	page, err := pageRepo.Create(ctx, scope, repository.CreatePageParams{
		Name:       "Revision Test Page",
		Title:      "Revision Test Page",
		Slug:       fmt.Sprintf("rev-test-page-%d", time.Now().UnixNano()),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	require.NoError(t, err)

	t.Run("AutosaveDraft", func(t *testing.T) {
		rev1, err := revisionSvc.AutosaveDraft(ctx, scope, service.AutosaveParams{
			PageID: page.ID,
			Snapshot: map[string]any{
				"title": "v1 title",
			},
			ChangeNote: "First draft",
			ActorID:    "11111111-1111-1111-1111-111111111111",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, rev1.ID)
		assert.Equal(t, 1, rev1.RevisionNumber)

		// Create another
		rev2, err := revisionSvc.AutosaveDraft(ctx, scope, service.AutosaveParams{
			PageID: page.ID,
			Snapshot: map[string]any{
				"title": "v2 title",
			},
			ChangeNote: "Second draft",
			ActorID:    "11111111-1111-1111-1111-111111111111",
		})
		require.NoError(t, err)
		assert.Equal(t, 2, rev2.RevisionNumber)
	})

	t.Run("List and Get Revisions", func(t *testing.T) {
		revs, err := revisionSvc.ListRevisions(ctx, scope, page.ID)
		require.NoError(t, err)
		assert.Len(t, revs, 2)
		assert.Equal(t, 2, revs[0].RevisionNumber) // descending order

		rev, err := revisionSvc.GetRevision(ctx, scope, revs[0].ID)
		require.NoError(t, err)
		assert.Equal(t, revs[0].ID, rev.ID)
	})

	t.Run("RestoreRevision", func(t *testing.T) {
		revs, err := revisionSvc.ListRevisions(ctx, scope, page.ID)
		require.NoError(t, err)

		restoredPage, err := revisionSvc.RestoreRevision(ctx, scope, revs[1].ID, "22222222-2222-2222-2222-222222222222") // Restore rev 1
		require.NoError(t, err)
		assert.Equal(t, domain.PageStatusDraft, restoredPage.Status)
		assert.Equal(t, "22222222-2222-2222-2222-222222222222", restoredPage.UpdatedBy)
	})

	t.Run("Schedule Actions", func(t *testing.T) {
		sch, err := revisionSvc.ScheduleAction(ctx, scope, service.SchedulePublishParams{
			PageID:      page.ID,
			Action:      domain.ScheduleActionPublish,
			ScheduledAt: time.Now().Add(1 * time.Hour),
			ActorID:     "11111111-1111-1111-1111-111111111111",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, sch.ID)

		schedules, err := revisionSvc.ListSchedules(ctx, scope, page.ID)
		require.NoError(t, err)
		assert.Len(t, schedules, 1)
		assert.Equal(t, sch.ID, schedules[0].ID)

		err = revisionSvc.CancelSchedule(ctx, scope, sch.ID)
		require.NoError(t, err)

		schedulesAfter, _ := revisionSvc.ListSchedules(ctx, scope, page.ID)
		assert.Len(t, schedulesAfter, 0)
	})
}
