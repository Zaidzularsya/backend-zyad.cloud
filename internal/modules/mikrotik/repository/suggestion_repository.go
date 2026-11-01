package repository

import (
	"github.com/jackc/pgx/v5"

	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

// Suggestion bases stored in mikrotik_secret_suggestions.basis.
const (
	SuggestionBasisPhone     = "phone"
	SuggestionBasisName      = "name"
	SuggestionBasisAmbiguous = "ambiguous"
)

// Suggestion is one computed pairing hint. AccountID is "" for an ambiguous
// row (the table requires NULL there); Candidates is >= 2 only for ambiguous.
type Suggestion struct {
	SecretID   string
	AccountID  string
	Basis      string
	Candidates int
}

type SuggestionRepository interface {
	// ReplaceForRouter deletes the router's old suggestions and inserts rows in
	// one transaction. rows may be empty (clears the router's suggestions).
	ReplaceForRouter(ctx context.Context, scope coretenant.Scope, routerID string, rows []Suggestion) error
}

type suggestionRepository struct{ db *database.Pool }

func NewSuggestionRepository(db *database.Pool) SuggestionRepository {
	return &suggestionRepository{db: db}
}

func (r *suggestionRepository) ReplaceForRouter(ctx context.Context, scope coretenant.Scope, routerID string, rows []Suggestion) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM mikrotik_secret_suggestions s
			USING mikrotik_ppp_secrets p
			WHERE s.organization_id = $1 AND p.organization_id = s.organization_id
			  AND p.id = s.secret_id AND p.router_id = $2`, scope.OrganizationID(), routerID); err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		secretIDs := make([]string, len(rows))
		accountIDs := make([]*string, len(rows))
		bases := make([]string, len(rows))
		cands := make([]int32, len(rows))
		for i, s := range rows {
			secretIDs[i], bases[i], cands[i] = s.SecretID, s.Basis, int32(s.Candidates)
			if s.AccountID != "" {
				id := s.AccountID
				accountIDs[i] = &id
			}
		}
		// The join on router_id keeps a foreign secret id from being inserted for this router.
		_, err := tx.Exec(ctx, `
			INSERT INTO mikrotik_secret_suggestions (organization_id, secret_id, account_id, basis, candidates, computed_at)
			SELECT $1::uuid, u.secret_id, u.account_id, u.basis, u.cands, now()
			FROM unnest($3::uuid[], $4::uuid[], $5::text[], $6::int[]) AS u(secret_id, account_id, basis, cands)
			JOIN mikrotik_ppp_secrets p ON p.organization_id = $1::uuid AND p.id = u.secret_id AND p.router_id = $2::uuid`,
			scope.OrganizationID(), routerID, secretIDs, accountIDs, bases, cands)
		return err
	})
}
