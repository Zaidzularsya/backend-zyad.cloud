package service

import (
	"context"
	"strconv"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

const (
	// statsDeadline bounds the whole live read (dial + three commands).
	statsDeadline = 8 * time.Second
	// statsCacheTTL keeps dashboard polling from hitting the router every time.
	statsCacheTTL = 60 * time.Second
)

type statsCacheEntry struct {
	stats   domain.RouterStats
	expires time.Time
}

// Stats reads identity, resource and active PPPoE session count from the
// router. A failure of dial or of the first command (/system/identity) fails
// the call and is never cached; later commands that fail only blank their own
// fields. It does not record a check result.
func (s *RouterService) Stats(ctx context.Context, scope coretenant.Scope, id string) (domain.RouterStats, error) {
	router, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.RouterStats{}, err
	}
	key := scope.OrganizationID() + "/" + router.ID
	if stats, ok := s.cachedStats(key); ok {
		return stats, nil
	}

	ctx, cancel := context.WithTimeout(ctx, statsDeadline)
	defer cancel()
	client, password, err := s.dial(ctx, router)
	if err != nil {
		return domain.RouterStats{}, err
	}

	stats := domain.RouterStats{}
	rows, err := client.Print(ctx, "/system/identity", nil)
	if err != nil {
		return domain.RouterStats{}, translateClientError(err, password, router.PasswordEncrypted)
	}
	if len(rows) > 0 {
		stats.Identity = rows[0]["name"]
	}
	if rows, err := client.Print(ctx, "/system/resource", nil); err == nil && len(rows) > 0 {
		r := rows[0]
		stats.BoardName, stats.Version, stats.Architecture, stats.Uptime = r["board-name"], r["version"], r["architecture-name"], r["uptime"]
		if v, err := strconv.Atoi(r["cpu-load"]); err == nil {
			stats.CPULoad = &v
		}
		if v, err := strconv.ParseInt(r["total-memory"], 10, 64); err == nil {
			stats.MemoryTotal = &v
		}
		if v, err := strconv.ParseInt(r["free-memory"], 10, 64); err == nil {
			stats.MemoryFree = &v
		}
	}
	if rows, err := client.Print(ctx, "/ppp/active", nil); err == nil {
		n := len(rows)
		stats.PPPActive = &n
	}

	now := s.now()
	stats.FetchedAt = now
	s.statsMu.Lock()
	s.statsCache[key] = statsCacheEntry{stats: stats, expires: now.Add(statsCacheTTL)}
	s.statsMu.Unlock()
	return stats, nil
}

func (s *RouterService) cachedStats(key string) (domain.RouterStats, bool) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	e, ok := s.statsCache[key]
	if !ok {
		return domain.RouterStats{}, false
	}
	if !s.now().Before(e.expires) {
		delete(s.statsCache, key)
		return domain.RouterStats{}, false
	}
	return e.stats, true
}
