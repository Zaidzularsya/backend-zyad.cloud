package redis

import (
	"context"
	"fmt"
	"log/slog"

	"zyad.cloud/internal/config"

	goredis "github.com/redis/go-redis/v9"
)

type Client struct {
	*goredis.Client
}

func Connect(ctx context.Context, cfg config.RedisConfig, logger *slog.Logger) (*Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
	})

	if logger != nil {
		logger.Info("redis connected", "host", cfg.Host, "port", cfg.Port)
	}

	return &Client{Client: client}, nil
}
