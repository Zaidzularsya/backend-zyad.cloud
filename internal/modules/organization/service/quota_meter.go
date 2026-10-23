package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"zyad.cloud/internal/core/businesstime"
)

// QuotaMeter counts current usage of a quota feature for one organization.
// periodStart nil means count everything; non-nil counts since that instant.
type QuotaMeter interface {
	Used(ctx context.Context, organizationID string, periodStart *time.Time) (int64, error)
}

// QuotaMeterRegistry maps a feature key to its QuotaMeter.
type QuotaMeterRegistry struct {
	mu     sync.RWMutex
	meters map[string]QuotaMeter
}

func NewQuotaMeterRegistry() *QuotaMeterRegistry {
	return &QuotaMeterRegistry{meters: map[string]QuotaMeter{}}
}

func (r *QuotaMeterRegistry) Register(featureKey string, m QuotaMeter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.meters[featureKey] = m
}

func (r *QuotaMeterRegistry) Get(featureKey string) (QuotaMeter, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.meters[featureKey]
	return m, ok
}

// PeriodStart returns the start of the counting window for a reset strategy.
// "never" has no window (nil); "monthly" starts at the first day of the
// current month in the business timezone. Other strategies are unsupported.
func PeriodStart(resetStrategy string, now time.Time) (*time.Time, error) {
	switch resetStrategy {
	case "never":
		return nil, nil
	case "monthly":
		local := now.In(businesstime.Location())
		start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, businesstime.Location())
		return &start, nil
	default:
		return nil, fmt.Errorf("reset strategy %q tidak didukung", resetStrategy)
	}
}
