package migration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"zyad.cloud/internal/platform/database"

	"github.com/jackc/pgx/v5"
)

type Direction string

const (
	DirectionUp   Direction = "up"
	DirectionDown Direction = "down"
)

type Migration struct {
	Version  string
	Name     string
	UpPath   string
	DownPath string
}

type Result struct {
	Version string
	Name    string
	Applied bool
}

var migrationFilePattern = regexp.MustCompile(`^([0-9]+)_(.+)\.(up|down)\.sql$`)

func Run(ctx context.Context, db *database.Pool, dir string, direction Direction, steps int) ([]Result, error) {
	if db == nil {
		return nil, errors.New("database pool is required")
	}
	if steps < 0 {
		return nil, errors.New("steps must be zero or greater")
	}

	migrations, err := Load(dir)
	if err != nil {
		return nil, err
	}

	switch direction {
	case DirectionUp:
		return runUp(ctx, db, migrations, steps)
	case DirectionDown:
		return runDown(ctx, db, migrations, steps)
	default:
		return nil, fmt.Errorf("unsupported migration direction: %s", direction)
	}
}

func Load(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	byVersion := map[string]Migration{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}

		version := matches[1]
		name := matches[2]
		direction := matches[3]

		migration := byVersion[version]
		migration.Version = version
		migration.Name = name
		path := filepath.Join(dir, entry.Name())

		if direction == string(DirectionUp) {
			migration.UpPath = path
		} else {
			migration.DownPath = path
		}
		byVersion[version] = migration
	}

	migrations := make([]Migration, 0, len(byVersion))
	for _, migration := range byVersion {
		if migration.UpPath == "" || migration.DownPath == "" {
			return nil, fmt.Errorf("migration %s_%s must have up and down files", migration.Version, migration.Name)
		}
		migrations = append(migrations, migration)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func runUp(ctx context.Context, db *database.Pool, migrations []Migration, steps int) ([]Result, error) {
	if err := ensureSchemaMigrations(ctx, db); err != nil {
		return nil, err
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0)
	for _, migration := range migrations {
		if _, ok := applied[migration.Version]; ok {
			continue
		}
		if steps > 0 && len(results) >= steps {
			break
		}
		if err := apply(ctx, db, migration, DirectionUp); err != nil {
			return results, err
		}
		results = append(results, Result{Version: migration.Version, Name: migration.Name, Applied: true})
	}

	return results, nil
}

func runDown(ctx context.Context, db *database.Pool, migrations []Migration, steps int) ([]Result, error) {
	if err := ensureSchemaMigrations(ctx, db); err != nil {
		return nil, err
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0)
	for i := len(migrations) - 1; i >= 0; i-- {
		migration := migrations[i]
		if _, ok := applied[migration.Version]; !ok {
			continue
		}
		if steps > 0 && len(results) >= steps {
			break
		}
		if err := apply(ctx, db, migration, DirectionDown); err != nil {
			return results, err
		}
		results = append(results, Result{Version: migration.Version, Name: migration.Name, Applied: true})
	}

	return results, nil
}

func ensureSchemaMigrations(ctx context.Context, db *database.Pool) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version varchar(32) PRIMARY KEY,
			name varchar(255) NOT NULL,
			applied_at timestamp without time zone NOT NULL DEFAULT now()
		)
	`)
	return err
}

func appliedVersions(ctx context.Context, db *database.Pool) (map[string]struct{}, error) {
	rows, err := db.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[string]struct{}{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = struct{}{}
	}

	return applied, rows.Err()
}

func apply(ctx context.Context, db *database.Pool, migration Migration, direction Direction) error {
	path := migration.UpPath
	if direction == DirectionDown {
		path = migration.DownPath
	}

	sql, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", path, err)
	}

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	statement := strings.TrimSpace(string(sql))
	if statement != "" {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("execute migration %s: %w", filepath.Base(path), err)
		}
	}

	if direction == DirectionUp {
		if _, err := tx.Exec(ctx, `
			INSERT INTO schema_migrations (version, name, applied_at)
			VALUES ($1, $2, now())
		`, migration.Version, migration.Name); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			DELETE FROM schema_migrations
			WHERE version = $1
		`, migration.Version); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
