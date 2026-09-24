package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// SendRateLimiter limits outbound messages per session.
type SendRateLimiter interface {
	// Allow reports whether one more message may be sent from the session in
	// the current minute.
	Allow(ctx context.Context, sessionID string) bool
}

type redisCounter interface {
	Incr(ctx context.Context, key string) *goredis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
}

// RedisSendRateLimiter counts messages per session and minute (fixed window,
// pattern: landing/service/visibility_service.go). It fails open: a Redis
// outage must not stop sales from replying, so errors are only logged.
type RedisSendRateLimiter struct {
	redis     redisCounter
	perMinute int64
	log       *slog.Logger
	now       func() time.Time
}

func NewRedisSendRateLimiter(redis redisCounter, perMinute int, log *slog.Logger) *RedisSendRateLimiter {
	if log == nil {
		log = slog.Default()
	}
	return &RedisSendRateLimiter{redis: redis, perMinute: int64(perMinute), log: log, now: time.Now}
}

func (l *RedisSendRateLimiter) Allow(ctx context.Context, sessionID string) bool {
	if l.perMinute <= 0 {
		return true
	}
	key := fmt.Sprintf("wa:rl:%s:%d", sessionID, l.now().Unix()/60)
	count, err := l.redis.Incr(ctx, key).Result()
	if err != nil {
		l.log.Warn("whatsapp: rate limiter unavailable, allowing send", "error", err)
		return true
	}
	if count == 1 {
		if err := l.redis.Expire(ctx, key, 2*time.Minute).Err(); err != nil {
			l.log.Warn("whatsapp: rate limiter expire failed", "error", err)
		}
	}
	return count <= l.perMinute
}
