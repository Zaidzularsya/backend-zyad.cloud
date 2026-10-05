package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
)

type fakeRateCounter struct {
	counts map[string]int64
	err    error
}

func (f *fakeRateCounter) Incr(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx)
	if f.err != nil {
		cmd.SetErr(f.err)
		return cmd
	}
	f.counts[key]++
	cmd.SetVal(f.counts[key])
	return cmd
}

func (f *fakeRateCounter) Expire(ctx context.Context, _ string, _ time.Duration) *goredis.BoolCmd {
	return goredis.NewBoolCmd(ctx)
}

func rateRouter(counter RateCounter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(counter, "rl:test:", 2, time.Minute, func(c *gin.Context) string { return c.ClientIP() }))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func hit(r http.Handler, ip string) int {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = ip + ":1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRateLimitBlocksOverLimitPerKey(t *testing.T) {
	r := rateRouter(&fakeRateCounter{counts: map[string]int64{}})
	for i := 0; i < 2; i++ {
		if code := hit(r, "203.0.113.1"); code != http.StatusOK {
			t.Fatalf("request %d = %d", i+1, code)
		}
	}
	if code := hit(r, "203.0.113.1"); code != http.StatusTooManyRequests {
		t.Fatalf("third request = %d, want 429", code)
	}
	if code := hit(r, "203.0.113.2"); code != http.StatusOK {
		t.Fatalf("other IP = %d, want 200", code)
	}
}

func TestRateLimitFailsOpen(t *testing.T) {
	r := rateRouter(&fakeRateCounter{counts: map[string]int64{}, err: errors.New("redis down")})
	for i := 0; i < 5; i++ {
		if code := hit(r, "203.0.113.1"); code != http.StatusOK {
			t.Fatalf("redis error must fail open, got %d", code)
		}
	}
	if code := hit(rateRouter(nil), "203.0.113.1"); code != http.StatusOK {
		t.Fatalf("nil counter must fail open, got %d", code)
	}
}
