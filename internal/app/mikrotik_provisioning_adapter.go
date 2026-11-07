package app

import (
	"context"
	"log/slog"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/service"
	receivablehandler "zyad.cloud/internal/modules/receivable/handler"
)

type mikrotikStager interface {
	Stage(ctx context.Context, scope coretenant.Scope, invoiceID string, paidAt, now, today time.Time) (service.Stage, bool, error)
}

// mikrotikProvisioningAdapter implements the receivable handler's
// PublicProvisioning port on top of the mikrotik public service.
type mikrotikProvisioningAdapter struct {
	stager mikrotikStager
	gate   service.PublicFeatureGate
	prof   service.PublicTenantProfile
	now    func() time.Time
}

func newMikrotikProvisioningAdapter(stager mikrotikStager, gate service.PublicFeatureGate, prof service.PublicTenantProfile, now func() time.Time) mikrotikProvisioningAdapter {
	if now == nil {
		now = time.Now
	}
	return mikrotikProvisioningAdapter{stager: stager, gate: gate, prof: prof, now: now}
}

// Stage returns Applicable=false (no error) when the tenant lacks mikrotik.enabled.
// A nil paidAt falls back to now, which keeps the stage at the conservative "received".
// For needs_help it also attaches the tenant contact; a profile failure only drops
// the contact, never the stage.
func (a mikrotikProvisioningAdapter) Stage(ctx context.Context, scope coretenant.Scope, invoiceID string, paidAt *time.Time) (receivablehandler.ProvisioningResult, error) {
	enabled, err := a.gate.Enabled(ctx, scope)
	if err != nil || !enabled {
		return receivablehandler.ProvisioningResult{}, err
	}
	now := a.now()
	paid := now
	if paidAt != nil {
		paid = *paidAt
	}
	stage, applicable, err := a.stager.Stage(ctx, scope, invoiceID, paid, now, businesstime.DayOf(now))
	if err != nil || !applicable {
		return receivablehandler.ProvisioningResult{}, err
	}
	res := receivablehandler.ProvisioningResult{Stage: string(stage), Applicable: true}
	if stage == service.StageNeedsHelp {
		c, perr := a.prof.Profile(ctx, scope)
		if perr != nil {
			slog.WarnContext(ctx, "mikrotik: tenant profile unavailable for needs_help contact", "error", perr)
		} else {
			res.Contact = &receivablehandler.PublicContact{Name: c.Name, Phone: c.Phone, Email: c.Email}
		}
	}
	return res, nil
}
