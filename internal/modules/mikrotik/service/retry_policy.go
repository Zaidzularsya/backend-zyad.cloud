package service

import (
	"errors"
	"time"

	"zyad.cloud/internal/modules/mikrotik"
)

// Task actions understood by the retry policy.
const (
	ActionIsolate = "isolate"
	ActionRestore = "restore"
)

// FailureKind separates errors worth retrying from those that need a human.
type FailureKind int

const (
	// FailTransient: retry later with backoff.
	FailTransient FailureKind = iota
	// FailPermanent: retrying cannot help; the task fails immediately.
	FailPermanent
)

// TaskFailure is a classified failure. Message is fixed text that is safe to
// show tenants: it never carries raw router/library text or credentials.
type TaskFailure struct {
	Kind    FailureKind
	Code    string
	Message string
}

// Classify maps an executor error to a TaskFailure. Unknown errors are
// transient so a surprising failure is retried rather than silently dropped.
func Classify(err error) TaskFailure {
	perm := func(code, msg string) TaskFailure {
		return TaskFailure{Kind: FailPermanent, Code: code, Message: msg}
	}
	switch {
	case errors.Is(err, mikrotik.ErrRouterNotFound):
		return perm("router_not_found", "Router tidak ditemukan.")
	case errors.Is(err, mikrotik.ErrRouterNotIsolirReady):
		return perm("router_not_ready", "Router belum siap untuk isolir.")
	case errors.Is(err, mikrotik.ErrAuthFailed):
		return perm("auth_failed", "Router menolak kredensial.")
	case errors.Is(err, mikrotik.ErrTLSUntrusted):
		return perm("tls_untrusted", "Sertifikat TLS router tidak dipercaya.")
	case errors.Is(err, mikrotik.ErrHostNotAllowed):
		return perm("host_not_allowed", "Alamat router tidak diizinkan.")
	case errors.Is(err, mikrotik.ErrSecretMissingOnRouter):
		return perm("secret_missing", "PPP secret tidak ada di router.")
	case errors.Is(err, mikrotik.ErrRestoreProfileMissing):
		return perm("profile_missing", "Profile asal tidak ada di router.")
	case errors.Is(err, mikrotik.ErrRestoreTargetInvalid):
		return perm("restore_target_invalid", "Profile asal tidak diketahui atau tidak valid.")
	case errors.Is(err, mikrotik.ErrUnreachable):
		return TaskFailure{Kind: FailTransient, Code: "unreachable", Message: "Router tidak dapat dijangkau; akan dicoba lagi."}
	default:
		return TaskFailure{Kind: FailTransient, Code: "unknown", Message: "Terjadi kegagalan tak terduga; akan dicoba lagi."}
	}
}

// Retry budgets. "attempts" everywhere below counts attempts already made,
// including the one that just failed (attempts=1 -> first backoff).
const (
	isolateMaxAttempts = 8
	restoreMaxAttempts = 12

	// A tenant is notified once a task keeps failing (or is exhausted).
	isolateNotifyAttempts = 5
	restoreNotifyAttempts = 3
)

// Backoff tables: the delay after the Nth failed attempt is table[N-1]; past
// the end of the table the last entry repeats.
var (
	isolateBackoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	restoreBackoff = []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute}
)

// NextAttempt returns when to retry after `attempts` failed attempts, and
// whether the budget is exhausted (attempts >= max; next is then zero). An
// unknown action is treated as isolate.
func NextAttempt(action string, attempts int, now time.Time) (next time.Time, exhausted bool) {
	table, max := isolateBackoff, isolateMaxAttempts
	if action == ActionRestore {
		table, max = restoreBackoff, restoreMaxAttempts
	}
	if attempts >= max {
		return time.Time{}, true
	}
	idx := attempts - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(table) {
		idx = len(table) - 1
	}
	return now.Add(table[idx]), false
}

// ShouldNotifyTenant reports whether the tenant should hear about a task that
// has failed `attempts` times. Restore notifies earlier because a customer is
// left without service. Exhaustion always notifies.
func ShouldNotifyTenant(action string, attempts int) bool {
	if action == ActionRestore {
		return attempts >= restoreNotifyAttempts || attempts >= restoreMaxAttempts
	}
	return attempts >= isolateNotifyAttempts || attempts >= isolateMaxAttempts
}
