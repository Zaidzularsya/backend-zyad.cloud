//go:build integration

package phone

import (
	"context"
	"testing"

	"zyad.cloud/internal/platform/database/testutil"
)

// TestNormalizeIDMatchesSQLIntegration keeps NormalizeID and the SQL function
// normalize_phone_id (migration 000125, used by the generated
// crm_leads/crm_contacts.phone_normalized columns) in sync.
func TestNormalizeIDMatchesSQLIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	for _, tc := range normalizeCases {
		var sqlValue *string
		if err := db.QueryRow(ctx, `SELECT normalize_phone_id($1)`, tc.Raw).Scan(&sqlValue); err != nil {
			t.Fatalf("normalize_phone_id(%q): %v", tc.Raw, err)
		}
		goValue, ok := NormalizeID(tc.Raw)

		gotSQL := ""
		if sqlValue != nil {
			gotSQL = *sqlValue
		}
		if gotSQL != goValue || (sqlValue != nil) != ok {
			t.Errorf("%q: SQL = %v, Go = (%q, %v)", tc.Raw, sqlValue, goValue, ok)
		}
	}
}
