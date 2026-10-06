// Package seeder menyiapkan data sistem CRM: bot Self-Serve dan pipeline-nya.
package seeder

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/auth"
	"zyad.cloud/internal/platform/database"
)

const (
	// SelfServeSystemKey menandai pipeline Self-Serve (crm_pipelines.system_key).
	SelfServeSystemKey = "self_serve"
	selfServeBotName   = "Self-Serve Bot"
)

type SelfServeSeedResult struct{ BotUserID, PipelineID string }

type stageSeed struct {
	name        string
	probability string
	won, lost   bool
}

var selfServeStages = []stageSeed{
	{name: "Checkout dimulai", probability: "10"},
	{name: "Menunggu pembayaran", probability: "60"},
	{name: "Won", probability: "100", won: true},
	{name: "Lost", probability: "0", lost: true},
}

// SeedSelfServe idempoten: membuat user bot (aktor otomatis untuk checkout
// self-serve) dan pipeline Self-Serve di org platform bila belum ada.
func SeedSelfServe(ctx context.Context, db *database.Pool, platformOrganizationID, botEmail string) (SelfServeSeedResult, error) {
	if db == nil {
		return SelfServeSeedResult{}, errors.New("database pool is required")
	}
	botEmail = strings.TrimSpace(botEmail)
	if platformOrganizationID == "" || botEmail == "" {
		return SelfServeSeedResult{}, errors.New("platform organization id and bot email are required")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return SelfServeSeedResult{}, err
	}
	defer tx.Rollback(ctx)

	botID, err := ensureBotUser(ctx, tx, botEmail)
	if err != nil {
		return SelfServeSeedResult{}, err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", platformOrganizationID); err != nil {
		return SelfServeSeedResult{}, err
	}
	pipelineID, err := ensurePipeline(ctx, tx, platformOrganizationID, botID)
	if err != nil {
		return SelfServeSeedResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SelfServeSeedResult{}, err
	}
	return SelfServeSeedResult{BotUserID: botID, PipelineID: pipelineID}, nil
}

func ensureBotUser(ctx context.Context, tx pgx.Tx, email string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL`, email).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find self-serve bot: %w", err)
	}

	// Password acak yang tidak pernah dicetak: bot tidak untuk login.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	hash, err := auth.HashPassword(base64.RawURLEncoding.EncodeToString(raw))
	if err != nil {
		return "", fmt.Errorf("hash bot password: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, status, email_verified_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'active', now(), now(), now())
		RETURNING id`, selfServeBotName, email, hash).Scan(&id); err != nil {
		return "", fmt.Errorf("create self-serve bot: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_profiles (user_id, created_at, updated_at) VALUES ($1, now(), now()) ON CONFLICT (user_id) DO NOTHING`, id); err != nil {
		return "", fmt.Errorf("create bot profile: %w", err)
	}
	return id, nil
}

func ensurePipeline(ctx context.Context, tx pgx.Tx, orgID, botID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM crm_pipelines WHERE organization_id = $1 AND system_key = $2 AND deleted_at IS NULL`,
		orgID, SelfServeSystemKey).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find self-serve pipeline: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO crm_pipelines (organization_id, name, is_default, system_key, created_by, updated_by)
		VALUES ($1, 'Self-Serve', false, $2, $3, $3) RETURNING id`, orgID, SelfServeSystemKey, botID).Scan(&id); err != nil {
		return "", fmt.Errorf("create self-serve pipeline: %w", err)
	}
	for pos, s := range selfServeStages {
		if _, err := tx.Exec(ctx, `
			INSERT INTO crm_pipeline_stages (organization_id, pipeline_id, name, position, probability, is_won, is_lost)
			VALUES ($1, $2, $3, $4, $5::numeric, $6, $7)`, orgID, id, s.name, pos, s.probability, s.won, s.lost); err != nil {
			return "", fmt.Errorf("create self-serve stage %q: %w", s.name, err)
		}
	}
	return id, nil
}
