package app

import (
	"context"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/service"
)

type mikrotikStager interface {
	Stage(ctx context.Context, scope coretenant.Scope, invoiceID string, paidAt, now, today time.Time) (service.Stage, bool, error)
}

// mikrotikProvisioningAdapter implements the receivable handler's
// PublicProvisioning port on top of the mikrotik public service.
type mikrotikProvisioningAdapter struct {
	stager mikrotikStager
	gate   service.PublicFeatureGate
	now    func() time.Time
}

func newMikrotikProvisioningAdapter(stager mikrotikStager, gate service.PublicFeatureGate, now func() time.Time) mikrotikProvisioningAdapter {
	if now == nil {
		now = time.Now
	}
	return mikrotikProvisioningAdapter{stager: stager, gate: gate, now: now}
}

// Stage returns applicable=false (no error) when the tenant lacks mikrotik.enabled.
// A nil paidAt falls back to now, which keeps the stage at the conservative "received".
func (a mikrotikProvisioningAdapter) Stage(ctx context.Context, scope coretenant.Scope, invoiceID string, paidAt *time.Time) (string, bool, error) {
	enabled, err := a.gate.Enabled(ctx, scope)
	if err != nil {
		return "", false, err
	}
	if !enabled {
		return "", false, nil
	}
	now := a.now()
	paid := now
	if paidAt != nil {
		paid = *paidAt
	}
	stage, applicable, err := a.stager.Stage(ctx, scope, invoiceID, paid, now, businesstime.DayOf(now))
	if err != nil || !applicable {
		return "", false, err
	}
	return string(stage), true, nil
}
