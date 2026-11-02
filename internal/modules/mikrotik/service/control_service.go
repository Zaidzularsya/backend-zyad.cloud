package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

const (
	// maxActivationExemptions bounds the distinct accounts exempted while activating.
	maxActivationExemptions = 500
	// pilotTestWindow is how long a pilot test keeps the pilot account isolated.
	pilotTestWindow = 10 * time.Minute
	// restoreAllPage is the ListIsolated page size of RestoreAll.
	restoreAllPage = 200
)

// ControlDeps are the collaborators of ControlService. Evaluator may be nil
// (the API process only builds one when the engine is not off); Now defaults
// to time.Now.
type ControlDeps struct {
	Control   repository.ControlRepository
	Secrets   repository.SecretRepository
	Tasks     repository.TaskRepository
	Accounts  AccountDirectory
	Audit     Auditor
	Evaluator *Evaluator
	Now       func() time.Time
}

// ControlState is the tenant control center view. Engine is the deployment
// kill switch (off|dry_run|on); EffectiveMode is what the engine really does:
// "off" or "dry_run" while the env says so, otherwise the tenant Mode.
type ControlState struct {
	Mode            string
	GraceDays       int
	PilotVerifiedAt *time.Time
	Engine          string
	EffectiveMode   string
	PilotCount      int
	TestActiveCount int
	IsolatedSecrets int
}

// ExemptionInput exempts customers while activating isolation. Until is an
// optional last exempt business day (inclusive).
type ExemptionInput struct {
	AccountIDs []string
	Reason     string
	Until      *time.Time
}

type SetModeInput struct {
	Mode       string
	Exemptions *ExemptionInput
}

// PilotView is one pilot account with its customer name. Verified tells that
// the organization's pilot verification happened during this account's
// latest test.
type PilotView struct {
	domain.PilotAccount
	AccountName string
	TestActive  bool
	Verified    bool
}

// RestoreAllResult counts RestoreAll outcomes: Skipped = not eligible, an
// active task or in cooldown; Failed = the enqueue itself errored. Aborted:
// the tenant left paused mode mid-run, so the remaining pages were not queued.
type RestoreAllResult struct {
	Enqueued, Skipped, Failed int
	Aborted                   bool
}

// ControlService is the tenant-facing control surface of the isolation engine.
type ControlService struct {
	d      ControlDeps
	engine string
}

func NewControlService(d ControlDeps, engineMode string) *ControlService {
	if d.Now == nil {
		d.Now = time.Now
	}
	switch engineMode {
	case ModeOn, ModeDryRun:
	default:
		engineMode = ModeOff
	}
	return &ControlService{d: d, engine: engineMode}
}

func (s *ControlService) State(ctx context.Context, scope coretenant.Scope) (ControlState, error) {
	ctl, err := s.d.Control.Get(ctx, scope)
	if err != nil {
		return ControlState{}, err
	}
	pilots, err := s.d.Control.ListPilot(ctx, scope)
	if err != nil {
		return ControlState{}, err
	}
	isolated, err := s.d.Secrets.CountIsolated(ctx, scope)
	if err != nil {
		return ControlState{}, err
	}
	st := ControlState{
		Mode: ctl.Mode, GraceDays: ctl.GraceDays, PilotVerifiedAt: ctl.PilotVerifiedAt,
		Engine: s.engine, EffectiveMode: s.engine, PilotCount: len(pilots), IsolatedSecrets: isolated,
	}
	if s.engine == ModeOn {
		st.EffectiveMode = ctl.Mode
	}
	now := s.d.Now()
	for _, p := range pilots {
		if testActive(p, now) {
			st.TestActiveCount++
		}
	}
	return st, nil
}

func testActive(p domain.PilotAccount, now time.Time) bool {
	return p.TestUntil != nil && p.TestUntil.After(now)
}

func (s *ControlService) SetGraceDays(ctx context.Context, scope coretenant.Scope, days int, actor string) (ControlState, error) {
	if days < 0 || days > domain.MaxGraceDays {
		return ControlState{}, invalid("grace days must be between 0 and 60")
	}
	before, err := s.d.Control.Get(ctx, scope)
	if err != nil {
		return ControlState{}, err
	}
	if _, err := s.d.Control.SetGraceDays(ctx, scope, days, actor); err != nil {
		return ControlState{}, err
	}
	s.auditControl(ctx, scope, "control.grace_days_changed", actor, scope.OrganizationID(),
		map[string]any{"target": "control", "from": before.GraceDays, "to": days})
	return s.State(ctx, scope)
}

func validTenantMode(mode string) bool {
	switch mode {
	case domain.ModeSimulation, domain.ModePilot, domain.ModeActive, domain.ModePaused:
		return true
	}
	return false
}

// SetMode changes the tenant isolation mode. active needs a verified pilot
// from any source mode. Exemptions are only accepted with active; every
// account is validated before anything is written, and the exemptions and the
// mode are written in one transaction.
func (s *ControlService) SetMode(ctx context.Context, scope coretenant.Scope, in SetModeInput, actor string) (ControlState, error) {
	if !validTenantMode(in.Mode) {
		return ControlState{}, fmt.Errorf("%w: unknown isolation mode", mikrotik.ErrModeInvalid)
	}
	if in.Exemptions != nil && in.Mode != domain.ModeActive {
		return ControlState{}, fmt.Errorf("%w: exemptions are only accepted when activating", mikrotik.ErrModeInvalid)
	}
	exemptions, err := s.exemptions(in.Exemptions)
	if err != nil {
		return ControlState{}, err
	}
	current, err := s.d.Control.Get(ctx, scope)
	if err != nil {
		return ControlState{}, err
	}
	if in.Mode == domain.ModeActive && current.PilotVerifiedAt == nil {
		return ControlState{}, mikrotik.ErrPilotRequired
	}
	if current.Mode == in.Mode && len(exemptions) == 0 {
		return s.State(ctx, scope)
	}
	// Accounts are resolved only after the cheap checks above.
	for _, e := range exemptions {
		if _, err := s.d.Accounts.Get(ctx, scope, e.AccountID); err != nil {
			return ControlState{}, err
		}
	}
	if len(exemptions) > 0 {
		_, err = s.d.Control.ActivateWithExemptions(ctx, scope, in.Mode, exemptions, actor)
	} else {
		_, err = s.d.Control.SetMode(ctx, scope, in.Mode, actor)
	}
	if err != nil {
		return ControlState{}, err
	}
	s.auditControl(ctx, scope, "control.mode_changed", actor, scope.OrganizationID(),
		map[string]any{"target": "control", "from": current.Mode, "to": in.Mode, "exemptions": len(exemptions)})
	for _, e := range exemptions {
		var until any
		if e.Until != nil {
			until = e.Until.Format("2006-01-02")
		}
		s.auditControl(ctx, scope, "customer_policy.exempt_set", actor, e.AccountID,
			map[string]any{"target": "customer_policy", "account_id": e.AccountID, "exempt": true, "reason": e.Reason, "exempted_until": until})
	}
	return s.State(ctx, scope)
}

// exemptions validates the input shape (not the accounts) and dedupes ids.
func (s *ControlService) exemptions(in *ExemptionInput) ([]repository.Exemption, error) {
	if in == nil || len(in.AccountIDs) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range in.AccountIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > maxActivationExemptions {
		return nil, invalid("at most 500 customers can be exempted at once")
	}
	reason := strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(reason); n == 0 || n > maxExemptReasonRunes {
		return nil, invalid("reason is required (1-500 characters) when exempting customers")
	}
	var until *time.Time
	if in.Until != nil {
		d := time.Date(in.Until.Year(), in.Until.Month(), in.Until.Day(), 0, 0, 0, 0, time.UTC)
		if d.Before(businesstime.DayOf(s.d.Now())) {
			return nil, invalid("exempted_until must not be in the past")
		}
		until = &d
	}
	out := make([]repository.Exemption, 0, len(ids))
	for _, id := range ids {
		out = append(out, repository.Exemption{AccountID: id, Reason: reason, Until: until})
	}
	return out, nil
}

// ListPilot returns the pilot accounts with their customer names (one
// directory lookup per pilot; an unknown account keeps an empty name).
func (s *ControlService) ListPilot(ctx context.Context, scope coretenant.Scope) ([]PilotView, error) {
	ctl, err := s.d.Control.Get(ctx, scope)
	if err != nil {
		return nil, err
	}
	pilots, err := s.d.Control.ListPilot(ctx, scope)
	if err != nil {
		return nil, err
	}
	now := s.d.Now()
	out := make([]PilotView, 0, len(pilots))
	for _, p := range pilots {
		v := PilotView{PilotAccount: p, TestActive: testActive(p, now)}
		v.Verified = ctl.PilotVerifiedAt != nil && p.TestStartedAt != nil && !ctl.PilotVerifiedAt.Before(*p.TestStartedAt)
		acc, err := s.d.Accounts.Get(ctx, scope, p.AccountID)
		switch {
		case err == nil:
			v.AccountName = acc.Name
		case !errors.Is(err, mikrotik.ErrAccountNotFound):
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *ControlService) AddPilot(ctx context.Context, scope coretenant.Scope, accountID, actor string) error {
	if _, err := s.d.Accounts.Get(ctx, scope, accountID); err != nil {
		return err
	}
	if err := s.d.Control.AddPilot(ctx, scope, accountID, actor); err != nil {
		return err
	}
	s.auditControl(ctx, scope, "pilot.added", actor, accountID, map[string]any{"target": "pilot", "account_id": accountID})
	return nil
}

// RemovePilot is idempotent. A running test is stopped first; afterwards the
// account is re-evaluated so a pilot isolation is restored right away.
func (s *ControlService) RemovePilot(ctx context.Context, scope coretenant.Scope, accountID, actor string) error {
	isPilot, active, err := s.d.Control.PilotState(ctx, scope, accountID)
	if err != nil {
		return err
	}
	if !isPilot {
		return nil
	}
	if active {
		if err := s.d.Control.StopPilotTest(ctx, scope, accountID); err != nil && !errors.Is(err, mikrotik.ErrAccountNotFound) {
			return err
		}
		s.auditControl(ctx, scope, "pilot.test_stopped", actor, accountID, map[string]any{"target": "pilot", "account_id": accountID})
	}
	if err := s.d.Control.RemovePilot(ctx, scope, accountID); err != nil {
		return err
	}
	s.auditControl(ctx, scope, "pilot.removed", actor, accountID, map[string]any{"target": "pilot", "account_id": accountID})
	s.reevaluate(ctx, scope, accountID, "pilot_removed")
	return nil
}

func errNotPilot() error { return invalid("the account is not in the pilot list") }

// StartPilotTest isolates one pilot account for pilotTestWindow. The window is
// written first; the evaluator then enqueues the isolate (the reconciler never
// visits an account without trigger invoices).
func (s *ControlService) StartPilotTest(ctx context.Context, scope coretenant.Scope, accountID, actor string) (time.Time, error) {
	if s.engine != ModeOn || s.d.Evaluator == nil {
		return time.Time{}, mikrotik.ErrEngineOff
	}
	ctl, err := s.d.Control.Get(ctx, scope)
	if err != nil {
		return time.Time{}, err
	}
	if ctl.Mode != domain.ModePilot {
		return time.Time{}, mikrotik.ErrPilotModeRequired
	}
	isPilot, _, err := s.d.Control.PilotState(ctx, scope, accountID)
	if err != nil {
		return time.Time{}, err
	}
	if !isPilot {
		return time.Time{}, errNotPilot()
	}
	secrets, err := s.d.Secrets.ListByAccount(ctx, scope, accountID)
	if err != nil {
		return time.Time{}, err
	}
	if len(secrets) == 0 {
		return time.Time{}, invalid("the pilot account has no paired PPP secret")
	}
	until := s.d.Now().Add(pilotTestWindow)
	if err := s.d.Control.StartPilotTest(ctx, scope, accountID, until); err != nil {
		if errors.Is(err, mikrotik.ErrAccountNotFound) {
			return time.Time{}, errNotPilot()
		}
		return time.Time{}, err
	}
	dec, err := s.d.Evaluator.Apply(ctx, scope, accountID, "pilot_test")
	if err != nil {
		s.abortPilotTest(ctx, scope, accountID, actor, "enqueue_failed")
		return time.Time{}, fmt.Errorf("enqueue pilot isolation: %w", err)
	}
	if reason := pilotNotRunnableReason(dec, secrets); reason != "" {
		s.abortPilotTest(ctx, scope, accountID, actor, reason)
		return time.Time{}, &mikrotik.PilotTestNotRunnableError{Reason: reason}
	}
	s.auditControl(ctx, scope, "pilot.test_started", actor, accountID,
		map[string]any{"target": "pilot", "account_id": accountID, "test_until": until.UTC().Format(time.RFC3339)})
	return until, nil
}

// pilotNotRunnableReason is "" when the test isolates something (an isolate
// was enqueued) or a secret of the account is already isolated; otherwise the
// mikrotik.PilotBlocked* class explaining why nothing would happen.
func pilotNotRunnableReason(dec Decision, secrets []domain.PPPSecret) string {
	for _, a := range dec.Secrets {
		if a.Action == actionIsolate {
			return ""
		}
	}
	for _, sec := range secrets {
		if sec.IsolationState == domain.IsolationIsolated {
			return ""
		}
	}
	switch {
	case dec.Exempt:
		return mikrotik.PilotBlockedExempt
	case !dec.IsolateAllowed:
		return mikrotik.PilotBlockedByMode
	default:
		return mikrotik.PilotBlockedNoUsableSecret
	}
}

// abortPilotTest ends a test window that will not isolate anything, so the UI
// does not show a running test that can never be verified.
func (s *ControlService) abortPilotTest(ctx context.Context, scope coretenant.Scope, accountID, actor, reason string) {
	if err := s.d.Control.StopPilotTest(ctx, scope, accountID); err != nil {
		slog.ErrorContext(ctx, "mikrotik: ending a failed pilot test window failed",
			"account_id", accountID, "reason", reason, "error", scrub(err.Error()))
	}
	s.auditControl(ctx, scope, "pilot.test_failed", actor, accountID,
		map[string]any{"target": "pilot", "account_id": accountID, "reason": reason})
}

// StopPilotTest ends the test window now and re-evaluates the account so the
// restore is enqueued immediately. Without a running test it is a no-op.
func (s *ControlService) StopPilotTest(ctx context.Context, scope coretenant.Scope, accountID, actor string) error {
	isPilot, active, err := s.d.Control.PilotState(ctx, scope, accountID)
	if err != nil {
		return err
	}
	if !isPilot {
		return errNotPilot()
	}
	if !active {
		return nil
	}
	if err := s.d.Control.StopPilotTest(ctx, scope, accountID); err != nil {
		if errors.Is(err, mikrotik.ErrAccountNotFound) {
			return errNotPilot()
		}
		return err
	}
	s.auditControl(ctx, scope, "pilot.test_stopped", actor, accountID, map[string]any{"target": "pilot", "account_id": accountID})
	s.reevaluate(ctx, scope, accountID, "pilot_test_stopped")
	return nil
}

// reevaluate applies the evaluator when one is wired. Failures are logged:
// the control change is committed and the periodic reconciler restores
// isolated secrets.
func (s *ControlService) reevaluate(ctx context.Context, scope coretenant.Scope, accountID, event string) {
	if s.d.Evaluator == nil {
		return
	}
	if _, err := s.d.Evaluator.Apply(ctx, scope, accountID, event); err != nil {
		slog.ErrorContext(ctx, "mikrotik: re-evaluation after control change failed",
			"account_id", accountID, "event", event, "error", scrub(err.Error()))
	}
}

// RestoreAll queues a restore for every isolated secret while the tenant is
// paused. The mode is re-read before every page: when the tenant leaves
// paused mid-run, the run stops with Aborted and a partial count. One failing
// enqueue is counted and does not stop the run; repeating the call skips
// secrets that already have an active task.
func (s *ControlService) RestoreAll(ctx context.Context, scope coretenant.Scope, actor string) (RestoreAllResult, error) {
	var res RestoreAllResult
	after := ""
	for first := true; ; first = false {
		ctl, err := s.d.Control.Get(ctx, scope)
		if first {
			if err != nil {
				return RestoreAllResult{}, err
			}
			if ctl.Mode != domain.ModePaused {
				return RestoreAllResult{}, mikrotik.ErrNotPaused
			}
		} else if err != nil || ctl.Mode != domain.ModePaused {
			if err != nil {
				slog.ErrorContext(ctx, "mikrotik: restore-all mode re-read failed; stopping", "error", scrub(err.Error()))
			}
			res.Aborted = true
			break
		}
		page, err := s.d.Secrets.ListIsolated(ctx, scope, after, restoreAllPage)
		if err != nil {
			return RestoreAllResult{}, err
		}
		for _, sec := range page {
			ok, qErr := s.d.Tasks.EnqueueRestore(ctx, scope, sec.ID, "restore_all")
			switch {
			case qErr != nil:
				res.Failed++
				slog.ErrorContext(ctx, "mikrotik: restore-all enqueue failed", "secret_id", sec.ID, "error", scrub(qErr.Error()))
			case ok:
				res.Enqueued++
			default:
				res.Skipped++
			}
		}
		if len(page) < restoreAllPage {
			break
		}
		after = page[len(page)-1].ID
	}
	s.auditControl(ctx, scope, "control.restore_all", actor, scope.OrganizationID(),
		map[string]any{"target": "control", "enqueued": res.Enqueued, "skipped": res.Skipped, "failed": res.Failed, "aborted": res.Aborted})
	return res, nil
}

// auditControl writes an audit row after a committed change; a failed audit
// row must not fail the change.
func (s *ControlService) auditControl(ctx context.Context, scope coretenant.Scope, event, actor, target string, md map[string]any) {
	if err := s.d.Audit.InsertAudit(ctx, scope, repository.AuditEntry{Event: event, ActorUserID: actor, TargetID: target, Metadata: md}); err != nil {
		slog.Warn("mikrotik: audit insert failed", "event", event, "target_id", target, "error", scrub(err.Error()))
	}
}
