package database

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"zyad.cloud/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct {
	*pgxpool.Pool
}

func Connect(ctx context.Context, cfg config.DatabaseConfig, logger *slog.Logger) (*Pool, error) {
	connString := buildDSN(cfg)
	poolCfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	if logger != nil {
		logger.Info("postgres connected", "host", cfg.Host, "db", cfg.Name, "schema", cfg.Schema)
	}

	return &Pool{Pool: pool}, nil
}

func buildDSN(cfg config.DatabaseConfig) string {
	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.Username, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   cfg.Name,
	}

	q := u.Query()
	q.Set("sslmode", sslMode)
	if cfg.Schema != "" {
		q.Set("search_path", cfg.Schema)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
