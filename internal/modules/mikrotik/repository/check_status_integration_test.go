//go:build integration

package repository_test

import (
	"testing"

	"zyad.cloud/internal/modules/mikrotik/service"
)

// TestRecordCheckAcceptsEveryServiceStatus stores every status the service can
// produce; the last_check_status CHECK constraint must accept all of them.
// service.CheckStatuses is kept in sync with the CheckStatus* constants by a
// unit test in the service package.
func TestRecordCheckAcceptsEveryServiceStatus(t *testing.T) {
	e := setupRouter(t)
	r := e.create(t, e.tenants.A.Scope, "Status Router")
	if len(service.CheckStatuses) == 0 {
		t.Fatal("no check statuses")
	}
	for _, status := range service.CheckStatuses {
		if err := e.repo.RecordCheck(e.ctx, e.tenants.A.Scope, r.ID, status, ""); err != nil {
			t.Fatalf("RecordCheck(%q): %v", status, err)
		}
		got, err := e.repo.FindByID(e.ctx, e.tenants.A.Scope, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.LastCheckStatus != status {
			t.Fatalf("stored status = %q, want %q", got.LastCheckStatus, status)
		}
	}
}
