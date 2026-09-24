package service

import (
	"context"
	"errors"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type fakeCounter struct {
	counts  map[string]int64
	expires int
	err     error
}

func (c *fakeCounter) Incr(_ context.Context, key string) *goredis.IntCmd {
	if c.err != nil {
		return goredis.NewIntResult(0, c.err)
	}
	c.counts[key]++
	return goredis.NewIntResult(c.counts[key], nil)
}

func (c *fakeCounter) Expire(context.Context, string, time.Duration) *goredis.BoolCmd {
	c.expires++
	return goredis.NewBoolResult(true, nil)
}

func TestRedisSendRateLimiter(t *testing.T) {
	counter := &fakeCounter{counts: map[string]int64{}}
	limiter := NewRedisSendRateLimiter(counter, 2, nil)
	limiter.now = func() time.Time { return time.Unix(1790000000, 0) }

	if !limiter.Allow(context.Background(), "s1") || !limiter.Allow(context.Background(), "s1") {
		t.Fatal("first two sends should be allowed")
	}
	if limiter.Allow(context.Background(), "s1") {
		t.Fatal("third send in the same minute should be limited")
	}
	if !limiter.Allow(context.Background(), "s2") {
		t.Fatal("limit is per session")
	}
	if counter.expires != 2 {
		t.Fatalf("expire calls = %d, want one per new window key", counter.expires)
	}

	limiter.now = func() time.Time { return time.Unix(1790000060, 0) }
	if !limiter.Allow(context.Background(), "s1") {
		t.Fatal("next minute should reset the window")
	}
}

func TestRedisSendRateLimiterFailsOpen(t *testing.T) {
	limiter := NewRedisSendRateLimiter(&fakeCounter{err: errors.New("redis down")}, 1, nil)
	if !limiter.Allow(context.Background(), "s1") {
		t.Fatal("limiter must allow sends when Redis is unavailable")
	}
}
