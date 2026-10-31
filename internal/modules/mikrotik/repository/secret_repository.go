package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/platform/database"
)

// upsertBatchSize bounds the rows sent in one INSERT statement.
const upsertBatchSize = 500

// ImportedSecret is one /ppp/secret entry read from a router.
// NeedsOriginalProfile is computed by the service (profile equals the router's
// isolir profile); the repository only ever raises the flag, never clears it.
type ImportedSecret struct {
	Name                 string
	Profile              string
	Comment              string
	Disabled             bool
	NeedsOriginalProfile bool
}

type ImportResult struct{ Created, Updated, Missing int }

// SecretFilter narrows List. Paired nil means both paired and unpaired.
type SecretFilter struct {
	RouterID string
	Paired   *bool
	Search   string
}

type SecretRepository interface {
	// UpsertImported stores the rows in one transaction. It never touches
	// account_id, original_profile or isolation_state, and never clears
	// needs_original_profile. Duplicate names in rows: the last one wins.
	UpsertImported(ctx context.Context, scope coretenant.Scope, routerID string, rows []ImportedSecret, seenAt time.Time) (ImportResult, error)
	// MarkMissing sets present=false for the router's present rows whose
	// last_seen_at is before seenAt (or NULL) and returns how many changed.
	MarkMissing(ctx context.Context, scope coretenant.Scope, routerID string, seenAt time.Time) (int, error)
	// FindByID returns mikrotik.ErrSecretNotFound when absent in the organization.
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.PPPSecret, error)
	List(ctx context.Context, scope coretenant.Scope, f SecretFilter, limit, offset int) ([]domain.PPPSecret, int64, error)
	// SetOriginalProfile stores the profile to restore and clears needs_original_profile.
	SetOriginalProfile(ctx context.Context, scope coretenant.Scope, id, profile string) error
}

type secretRepository struct {
	db *database.Pool
}

func NewSecretRepository(db *database.Pool) SecretRepository {
	return &secretRepository{db: db}
}

const secretColumns = `
	id, organization_id, router_id, name, profile, original_profile, needs_original_profile, disabled, comment,
	present, last_seen_at, isolation_state, account_id, paired_by, paired_at, imported_at, created_at, updated_at
`

func scanSecret(row pgx.Row) (domain.PPPSecret, error) {
	var s domain.PPPSecret
	var original *string
	err := row.Scan(
		&s.ID, &s.OrganizationID, &s.RouterID, &s.Name, &s.Profile, &original, &s.NeedsOriginalProfile, &s.Disabled, &s.Comment,
		&s.Present, &s.LastSeenAt, &s.IsolationState, &s.AccountID, &s.PairedBy, &s.PairedAt, &s.ImportedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return domain.PPPSecret{}, err
	}
	s.OriginalProfile = derefString(original)
	return s, nil
}

func mapSecretError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return mikrotik.ErrSecretNotFound
	}
	return err
}

// upsertSecretsSQL binds unnest()ed arrays so a batch is a single statement
// with a constant number of parameters. On conflict only router-owned columns
// change; pairing and isolation state stay untouched. xmax = 0 identifies a
// freshly inserted row.
const upsertSecretsSQL = `
	INSERT INTO mikrotik_ppp_secrets AS s (
		organization_id, router_id, name, profile, disabled, comment, needs_original_profile,
		present, last_seen_at, imported_at, created_at, updated_at
	)
	SELECT $1::uuid, $2::uuid, u.name, u.profile, u.disabled, u.comment, u.needs, true, $8::timestamp, now(), now(), now()
	FROM unnest($3::text[], $4::text[], $5::boolean[], $6::text[], $7::boolean[]) AS u(name, profile, disabled, comment, needs)
	ON CONFLICT (router_id, name) DO UPDATE SET
		profile = EXCLUDED.profile,
		disabled = EXCLUDED.disabled,
		comment = EXCLUDED.comment,
		present = true,
		last_seen_at = EXCLUDED.last_seen_at,
		needs_original_profile = s.needs_original_profile
			OR (EXCLUDED.needs_original_profile AND COALESCE(s.original_profile, '') = ''),
		updated_at = now()
	RETURNING (xmax = 0)`

func (r *secretRepository) UpsertImported(ctx context.Context, scope coretenant.Scope, routerID string, rows []ImportedSecret, seenAt time.Time) (ImportResult, error) {
	rows = dedupeImported(rows)
	var res ImportResult
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		for start := 0; start < len(rows); start += upsertBatchSize {
			end := start + upsertBatchSize
			if end > len(rows) {
				end = len(rows)
			}
			batch := rows[start:end]
			names := make([]string, len(batch))
			profiles := make([]string, len(batch))
			disabled := make([]bool, len(batch))
			comments := make([]string, len(batch))
			needs := make([]bool, len(batch))
			for i, b := range batch {
				names[i], profiles[i], disabled[i], comments[i], needs[i] = b.Name, b.Profile, b.Disabled, b.Comment, b.NeedsOriginalProfile
			}
			out, err := tx.Query(ctx, upsertSecretsSQL, scope.OrganizationID(), routerID, names, profiles, disabled, comments, needs, seenAt.UTC())
			if err != nil {
				return err
			}
			for out.Next() {
				var inserted bool
				if err := out.Scan(&inserted); err != nil {
					out.Close()
					return err
				}
				if inserted {
					res.Created++
				} else {
					res.Updated++
				}
			}
			out.Close()
			if err := out.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, err
	}
	return res, nil
}

// dedupeImported keeps the last occurrence of each name, because a single
// INSERT ... ON CONFLICT cannot affect the same row twice.
func dedupeImported(rows []ImportedSecret) []ImportedSecret {
	index := make(map[string]int, len(rows))
	out := make([]ImportedSecret, 0, len(rows))
	for _, row := range rows {
		if i, ok := index[row.Name]; ok {
			out[i] = row
			continue
		}
		index[row.Name] = len(out)
		out = append(out, row)
	}
	return out
}

func (r *secretRepository) MarkMissing(ctx context.Context, scope coretenant.Scope, routerID string, seenAt time.Time) (int, error) {
	var n int
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE mikrotik_ppp_secrets
			SET present = false, updated_at = now()
			WHERE organization_id = $1 AND router_id = $2 AND present
			  AND (last_seen_at IS NULL OR last_seen_at < $3::timestamp)`,
			scope.OrganizationID(), routerID, seenAt.UTC())
		if err != nil {
			return err
		}
		n = int(tag.RowsAffected())
		return nil
	})
	return n, err
}

func (r *secretRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.PPPSecret, error) {
	var out domain.PPPSecret
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var scanErr error
		out, scanErr = scanSecret(tx.QueryRow(ctx,
			`SELECT `+secretColumns+` FROM mikrotik_ppp_secrets WHERE id = $1 AND organization_id = $2`,
			id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.PPPSecret{}, mapSecretError(err)
	}
	return out, nil
}

// likeEscaper escapes LIKE wildcards so a search term is matched literally.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (r *secretRepository) List(ctx context.Context, scope coretenant.Scope, f SecretFilter, limit, offset int) ([]domain.PPPSecret, int64, error) {
	limit = clampLimit(limit)
	if offset < 0 {
		offset = 0
	}
	where := []string{"organization_id = $1"}
	args := []interface{}{scope.OrganizationID()}
	if f.RouterID != "" {
		args = append(args, f.RouterID)
		where = append(where, fmt.Sprintf("router_id = $%d", len(args)))
	}
	if f.Paired != nil {
		if *f.Paired {
			where = append(where, "account_id IS NOT NULL")
		} else {
			where = append(where, "account_id IS NULL")
		}
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		args = append(args, "%"+likeEscaper.Replace(q)+"%")
		where = append(where, fmt.Sprintf(`(name ILIKE $%d OR comment ILIKE $%d)`, len(args), len(args)))
	}
	cond := strings.Join(where, " AND ")

	items := []domain.PPPSecret{}
	var total int64
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM mikrotik_ppp_secrets WHERE `+cond, args...).Scan(&total); err != nil {
			return err
		}
		pageArgs := append(append([]interface{}{}, args...), limit, offset)
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT `+secretColumns+` FROM mikrotik_ppp_secrets WHERE `+cond+`
			ORDER BY lower(name), id LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2), pageArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, scanErr := scanSecret(rows)
			if scanErr != nil {
				return scanErr
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *secretRepository) SetOriginalProfile(ctx context.Context, scope coretenant.Scope, id, profile string) error {
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE mikrotik_ppp_secrets
			SET original_profile = $1, needs_original_profile = false, updated_at = now()
			WHERE id = $2 AND organization_id = $3`,
			nullableString(profile), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
	return mapSecretError(err)
}
