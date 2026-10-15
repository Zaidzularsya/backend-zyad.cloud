//go:build integration

package repository_test

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

// createTestPipeline membuat pipeline 4 stage: Baru(0) Survei(1) Won(2) Lost(3).
func createTestPipeline(t *testing.T, db *database.Pool, scope coretenant.Scope, name string, isDefault bool) (pipelineID string, stageIDs []string) {
	t.Helper()
	p, err := repository.NewPipelineRepository(db).Create(context.Background(), scope, repository.CreatePipelineParams{
		Name: name, IsDefault: isDefault,
		Stages: []repository.StageInput{
			{Name: "Baru", Position: 0, Probability: "10"},
			{Name: "Survei", Position: 1, Probability: "40"},
			{Name: "Won", Position: 2, Probability: "100", IsWon: true},
			{Name: "Lost", Position: 3, Probability: "0", IsLost: true},
		},
	})
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	for _, s := range p.Stages {
		stageIDs = append(stageIDs, s.ID)
	}
	return p.ID, stageIDs
}

func TestDealRequirementFieldsRoundTrip(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	pipelineID, stages := createTestPipeline(t, db, tenants.A.Scope, "Utama", true)

	deals := repository.NewDealRepository(db)
	d, err := deals.Create(ctx, tenants.A.Scope, repository.CreateDealParams{
		PipelineID: pipelineID, StageID: stages[0], Title: "Internet kantor",
		Description: "50 Mbps untuk 10 user", DecisionMaker: "Bu Rina",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Description != "50 Mbps untuk 10 user" || d.DecisionMaker != "Bu Rina" {
		t.Fatalf("create: %+v", d)
	}
	empty := ""
	maker := "Pak Budi"
	updated, err := deals.Update(ctx, tenants.A.Scope, d.ID, repository.UpdateDealParams{Description: &empty, DecisionMaker: &maker})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "" || updated.DecisionMaker != "Pak Budi" {
		t.Fatalf("update: %+v", updated)
	}
}
