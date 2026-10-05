package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	"zyad.cloud/internal/shared/response"
)

// RateCounter adalah subset klien Redis untuk pembatas fixed-window.
type RateCounter interface {
	Incr(ctx context.Context, key string) *goredis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
}

// RateLimit membatasi request per kunci (prefix + keyFn(c)) dalam jendela tetap.
// Fail open: Redis mati atau counter nil tidak boleh menutup endpoint publik;
// kegagalan hanya dicatat. Melewati batas → 429 RATE_LIMITED.
func RateLimit(counter RateCounter, prefix string, limit int, window time.Duration, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if counter == nil || limit <= 0 {
			c.Next()
			return
		}
		// Bucket waktu pada key menjamin jendela selalu berakhir walau Expire gagal.
		bucket := time.Now().Unix() / int64(window.Seconds())
		key := fmt.Sprintf("%s%s:%d", prefix, keyFn(c), bucket)
		count, err := counter.Incr(c.Request.Context(), key).Result()
		if err != nil {
			slog.Warn("rate limiter unavailable, allowing request", "prefix", prefix, "error", err)
			c.Next()
			return
		}
		if count == 1 {
			if err := counter.Expire(c.Request.Context(), key, 2*window).Err(); err != nil {
				slog.Warn("rate limiter expire failed", "prefix", prefix, "error", err)
			}
		}
		if count > int64(limit) {
			response.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak permintaan. Coba lagi nanti.")
			c.Abort()
			return
		}
		c.Next()
	}
}
