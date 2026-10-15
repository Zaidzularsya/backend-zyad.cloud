package service

import (
	"context"
	"log/slog"
	"sync"

	coretenant "zyad.cloud/internal/core/tenant"
)

// Registry dibagikan ke semua service receivable. Panic listener ditelan
// (dicatat) supaya tidak menggagalkan request yang sudah ter-commit.
type Registry struct {
	mu        sync.RWMutex
	listeners []Listener
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) Add(l Listener) {
	if r == nil || l == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listeners = append(r.listeners, l)
}

func (r *Registry) snapshot() []Listener {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Listener(nil), r.listeners...)
}

func (r *Registry) InvoicePaid(ctx context.Context, scope coretenant.Scope, inv InvoiceRef) {
	for _, l := range r.snapshot() {
		safely("invoice paid", func() { l.InvoicePaid(ctx, scope, inv) })
	}
}

func (r *Registry) ContractCreated(ctx context.Context, scope coretenant.Scope, c ContractRef) {
	for _, l := range r.snapshot() {
		safely("contract created", func() { l.ContractCreated(ctx, scope, c) })
	}
}

func (r *Registry) ContractEnded(ctx context.Context, scope coretenant.Scope, c ContractRef) {
	for _, l := range r.snapshot() {
		safely("contract ended", func() { l.ContractEnded(ctx, scope, c) })
	}
}

func safely(what string, fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("receivable listener panicked", "event", what, "panic", rec)
		}
	}()
	fn()
}
