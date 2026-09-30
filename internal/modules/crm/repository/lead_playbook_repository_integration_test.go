//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/platform/database/testutil"
)

func TestDefaultLeadPlaybookSeeded(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	var steps, outcomes int
	err := db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM crm_playbook_steps s JOIN crm_playbooks p ON p.id = s.playbook_id
				WHERE p.key = 'default_lead_sop' AND p.organization_id IS NULL),
			(SELECT COUNT(*) FROM crm_playbook_outcomes o
				JOIN crm_playbook_steps s ON s.id = o.step_id
				JOIN crm_playbooks p ON p.id = s.playbook_id
				WHERE p.key = 'default_lead_sop' AND p.organization_id IS NULL)`).Scan(&steps, &outcomes)
	if err != nil {
		t.Fatalf("query seed: %v", err)
	}
	if steps != 2 || outcomes != 8 {
		t.Fatalf("steps=%d outcomes=%d, want 2 and 8", steps, outcomes)
	}
}
