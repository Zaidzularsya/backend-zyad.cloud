package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/platform/database"
)

// Exemption is one customer exempted while activating isolation. Until is a
// calendar date (UTC midnight); nil means no expiry.
type Exemption struct {
	AccountID string
	Until     *time.Time
	Reason    string
}

type ControlRepository interface {
	// Get returns simulation/0 defaults (without inserting) when no row exists.
	Get(ctx context.Context, scope coretenant.Scope) (domain.ControlSettings, error)
	SetGraceDays(ctx context.Context, scope coretenant.Scope, days int, by string) (domain.ControlSettings, error)
	SetMode(ctx context.Context, scope coretenant.Scope, mode, by string) (domain.ControlSettings, error)
	// ActivateWithExemptions upserts all exemptions and sets the mode in one
	// transaction; any unknown account fails the whole call (ErrAccountNotFound).
	ActivateWithExemptions(ctx context.Context, scope coretenant.Scope, mode string, ex []Exemption, by string) (domain.ControlSettings, error)
	// ListPilot is ordered by added_at, account_id.
	ListPilot(ctx context.Context, scope coretenant.Scope) ([]domain.PilotAccount, error)
	// PilotState: testActive = test_until > DB now().
	PilotState(ctx context.Context, scope coretenant.Scope, accountID string) (isPilot, testActive bool, err error)
	// AddPilot is idempotent; ErrAccountNotFound for an account outside the organization.
	AddPilot(ctx context.Context, scope coretenant.Scope, accountID, by string) error
	RemovePilot(ctx context.Context, scope coretenant.Scope, accountID string) error
	// StartPilotTest sets test_started_at=now() and test_until=until; ErrAccountNotFound if not a pilot.
	StartPilotTest(ctx context.Context, scope coretenant.Scope, accountID string, until time.Time) error
	// StopPilotTest sets test_until=now(); ErrAccountNotFound if not a pilot.
	StopPilotTest(ctx context.Context, scope coretenant.Scope, accountID string) error
	// MarkPilotRoundTrip records pilot_verified_at (once) when a successful
	// isolate task finished after the pilot test started; false means nothing was written.
	MarkPilotRoundTrip(ctx context.Context, scope coretenant.Scope, accountID string) (verified bool, err error)
}

type controlRepository struct {
	db *database.Pool
}

func NewControlRepository(db *database.Pool) ControlRepository {
	return &controlRepository{db: db}
}

const controlColumns = `organization_id, isolation_mode, grace_days, pilot_verified_at, updated_at`

func scanControl(row pgx.Row) (domain.ControlSettings, error) {
	var s domain.ControlSettings
	err := row.Scan(&s.OrganizationID, &s.Mode, &s.GraceDays, &s.PilotVerifiedAt, &s.UpdatedAt)
	return s, err
}

func validMode(mode string) bool {
	switch mode {
	case domain.ModeSimulation, domain.ModePilot, domain.ModeActive, domain.ModePaused:
		return true
	}
	return false
}

func isFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func (r *controlRepository) Get(ctx context.Context, scope coretenant.Scope) (domain.ControlSettings, error) {
	out := domain.ControlSettings{OrganizationID: scope.OrganizationID(), Mode: domain.ModeSimulation}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		s, err := scanControl(tx.QueryRow(ctx,
			`SELECT `+controlColumns+` FROM mikrotik_org_settings WHERE organization_id = $1`, scope.OrganizationID()))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		out = s
		return err
	})
	return out, err
}

func (r *controlRepository) SetGraceDays(ctx context.Context, scope coretenant.Scope, days int, by string) (domain.ControlSettings, error) {
	if days < 0 || days > domain.MaxGraceDays {
		return domain.ControlSettings{}, &mikrotik.InvalidInputError{Message: "grace days must be between 0 and 60"}
	}
	var out domain.ControlSettings
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		s, err := scanControl(tx.QueryRow(ctx, `
			INSERT INTO mikrotik_org_settings (organization_id, grace_days, updated_by, updated_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (organization_id) DO UPDATE
			SET grace_days = EXCLUDED.grace_days, updated_by = EXCLUDED.updated_by, updated_at = now()
			RETURNING `+controlColumns, scope.OrganizationID(), days, nullableString(by)))
		out = s
		return err
	})
	return out, err
}

func setModeTx(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, mode, by string) (domain.ControlSettings, error) {
	return scanControl(tx.QueryRow(ctx, `
		INSERT INTO mikrotik_org_settings (organization_id, isolation_mode, updated_by, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (organization_id) DO UPDATE
		SET isolation_mode = EXCLUDED.isolation_mode, updated_by = EXCLUDED.updated_by, updated_at = now()
		RETURNING `+controlColumns, scope.OrganizationID(), mode, nullableString(by)))
}

func (r *controlRepository) SetMode(ctx context.Context, scope coretenant.Scope, mode, by string) (domain.ControlSettings, error) {
	if !validMode(mode) {
		return domain.ControlSettings{}, &mikrotik.InvalidInputError{Message: "invalid isolation mode"}
	}
	var out domain.ControlSettings
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		s, err := setModeTx(ctx, tx, scope, mode, by)
		out = s
		return err
	})
	return out, err
}

func (r *controlRepository) ActivateWithExemptions(ctx context.Context, scope coretenant.Scope, mode string, ex []Exemption, by string) (domain.ControlSettings, error) {
	if !validMode(mode) {
		return domain.ControlSettings{}, &mikrotik.InvalidInputError{Message: "invalid isolation mode"}
	}
	var out domain.ControlSettings
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		for _, e := range ex {
			if _, err := tx.Exec(ctx, `
				INSERT INTO mikrotik_customer_policies (organization_id, account_id, exempt, reason, exempted_until, set_by, set_at)
				VALUES ($1, $2, true, $3, $4::date, $5, now())
				ON CONFLICT (organization_id, account_id) DO UPDATE
				SET exempt = true, reason = EXCLUDED.reason, exempted_until = EXCLUDED.exempted_until,
				    set_by = EXCLUDED.set_by, set_at = now()`,
				scope.OrganizationID(), e.AccountID, e.Reason, dateOrNil(e.Until), nullableString(by)); err != nil {
				return err
			}
		}
		s, err := setModeTx(ctx, tx, scope, mode, by)
		out = s
		return err
	})
	if isFKViolation(err) {
		return domain.ControlSettings{}, mikrotik.ErrAccountNotFound
	}
	if err != nil {
		return domain.ControlSettings{}, err
	}
	return out, nil
}

func (r *controlRepository) ListPilot(ctx context.Context, scope coretenant.Scope) ([]domain.PilotAccount, error) {
	var out []domain.PilotAccount
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT account_id, added_at, test_started_at, test_until
			FROM mikrotik_pilot_accounts WHERE organization_id = $1
			ORDER BY added_at, account_id`, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p domain.PilotAccount
			if err := rows.Scan(&p.AccountID, &p.AddedAt, &p.TestStartedAt, &p.TestUntil); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

func (r *controlRepository) PilotState(ctx context.Context, scope coretenant.Scope, accountID string) (bool, bool, error) {
	var isPilot, active bool
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT true, COALESCE(test_until > now(), false)
			FROM mikrotik_pilot_accounts WHERE organization_id = $1 AND account_id = $2`,
			scope.OrganizationID(), accountID).Scan(&isPilot, &active)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return isPilot, active, err
}

func (r *controlRepository) AddPilot(ctx context.Context, scope coretenant.Scope, accountID, by string) error {
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO mikrotik_pilot_accounts (organization_id, account_id, added_by, added_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (organization_id, account_id) DO NOTHING`,
			scope.OrganizationID(), accountID, nullableString(by))
		return err
	})
	if isFKViolation(err) {
		return mikrotik.ErrAccountNotFound
	}
	return err
}

func (r *controlRepository) RemovePilot(ctx context.Context, scope coretenant.Scope, accountID string) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`DELETE FROM mikrotik_pilot_accounts WHERE organization_id = $1 AND account_id = $2`,
			scope.OrganizationID(), accountID)
		return err
	})
}

func (r *controlRepository) updatePilot(ctx context.Context, scope coretenant.Scope, query string, args ...interface{}) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, args...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return mikrotik.ErrAccountNotFound
		}
		return nil
	})
}

func (r *controlRepository) StartPilotTest(ctx context.Context, scope coretenant.Scope, accountID string, until time.Time) error {
	return r.updatePilot(ctx, scope, `
		UPDATE mikrotik_pilot_accounts SET test_started_at = now(), test_until = $3
		WHERE organization_id = $1 AND account_id = $2`,
		scope.OrganizationID(), accountID, until.UTC())
}

func (r *controlRepository) StopPilotTest(ctx context.Context, scope coretenant.Scope, accountID string) error {
	return r.updatePilot(ctx, scope, `
		UPDATE mikrotik_pilot_accounts SET test_until = now()
		WHERE organization_id = $1 AND account_id = $2`,
		scope.OrganizationID(), accountID)
}

func (r *controlRepository) MarkPilotRoundTrip(ctx context.Context, scope coretenant.Scope, accountID string) (bool, error) {
	verified := false
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT true FROM mikrotik_pilot_accounts p
			WHERE p.organization_id = $1 AND p.account_id = $2 AND p.test_started_at IS NOT NULL
			  AND EXISTS (
				SELECT 1 FROM mikrotik_isolation_tasks t
				JOIN mikrotik_ppp_secrets s ON s.organization_id = t.organization_id AND s.id = t.secret_id
				WHERE t.organization_id = p.organization_id AND s.account_id = p.account_id
				  AND t.action = 'isolate' AND t.status = 'done' AND t.finished_at >= p.test_started_at)`,
			scope.OrganizationID(), accountID).Scan(&verified)
		if errors.Is(err, pgx.ErrNoRows) {
			verified = false
			return nil
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO mikrotik_org_settings (organization_id, pilot_verified_at, updated_at)
			VALUES ($1, now(), now())
			ON CONFLICT (organization_id) DO UPDATE
			SET pilot_verified_at = COALESCE(mikrotik_org_settings.pilot_verified_at, now()), updated_at = now()`,
			scope.OrganizationID())
		return err
	})
	if err != nil {
		return false, err
	}
	return verified, nil
}
