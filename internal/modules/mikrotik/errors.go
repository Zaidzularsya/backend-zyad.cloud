package mikrotik

import "errors"

var (
	ErrRouterNotFound  = errors.New("mikrotik router not found")
	ErrRouterNameTaken = errors.New("mikrotik router name already used in this organization")

	// ErrHostNotAllowed: the host resolves outside the public range and the
	// configured private allow list.
	ErrHostNotAllowed = errors.New("mikrotik router host is not allowed")
	// ErrTLSRequired: a publicly reachable router must be contacted over TLS.
	ErrTLSRequired = errors.New("mikrotik router with a public host must use TLS")
	// ErrAuthFailed: the router rejected the credentials.
	ErrAuthFailed = errors.New("mikrotik router rejected the credentials")
	// ErrUnreachable: DNS failure, connection refused, or timeout.
	ErrUnreachable = errors.New("mikrotik router is unreachable")
	// ErrTLSUntrusted matches *TLSUntrustedError.
	ErrTLSUntrusted = errors.New("mikrotik router tls certificate is not trusted")
	// ErrInvalidInput matches *InvalidInputError.
	ErrInvalidInput = errors.New("invalid mikrotik router input")

	// ErrPlanChanged: the router state (or setup input) no longer produces the
	// previewed plan; nothing was written.
	ErrPlanChanged = errors.New("mikrotik setup plan changed since preview")
	// ErrSetupConflict matches *SetupConflictError.
	ErrSetupConflict = errors.New("mikrotik setup conflicts with existing router configuration")
	// ErrSetupFailed matches *SetupStepError whose cause is not a transport error.
	ErrSetupFailed = errors.New("mikrotik setup step failed")
	// ErrSetupStateNotSaved: the router objects WERE written (or removed) but
	// recording isolir_ready failed. Repeating the same call is idempotent.
	ErrSetupStateNotSaved = errors.New("router objects were changed but the isolir state was not saved; repeat the same operation (it is idempotent)")
)

// ErrSetupMarkFailed: isolir_ready could not be cleared before a change, so
// nothing was changed on the router (fail closed).
var ErrSetupMarkFailed = errors.New("could not record the isolir state; nothing was changed on the router")

// SetupConflictError names a pre-existing object that setup refuses to touch
// (or a state it cannot repair), so the operator can resolve it manually.
type SetupConflictError struct {
	Kind   string
	Name   string
	Detail string
}

func (e *SetupConflictError) Error() string {
	msg := "mikrotik setup conflict: " + e.Kind
	if e.Name != "" {
		msg += " " + e.Name
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

func (e *SetupConflictError) Is(target error) bool { return target == ErrSetupConflict }

// SetupStepError reports the router object a setup step failed on. Cause is a
// mapped sentinel (unreachable, auth, TLS) or ErrSetupFailed; raw router text
// is never carried.
type SetupStepError struct {
	Step  string // e.g. "create /ppp/profile"
	Cause error
	// RouterWritten is true when earlier steps already changed the router.
	RouterWritten bool
}

func (e *SetupStepError) Error() string {
	return "mikrotik setup failed at " + e.Step + ": " + e.Cause.Error()
}

func (e *SetupStepError) Unwrap() error { return e.Cause }

// TLSUntrustedError carries the leaf certificate SHA-256 (lowercase hex) so the
// UI can offer to pin it. PinMismatch is true when a pin was configured and the
// router presented a different certificate.
type TLSUntrustedError struct {
	Fingerprint string
	PinMismatch bool
}

func (e *TLSUntrustedError) Error() string { return ErrTLSUntrusted.Error() }

func (e *TLSUntrustedError) Is(target error) bool { return target == ErrTLSUntrusted }

// InvalidInputError is a validation failure with a message safe to show to users.
type InvalidInputError struct {
	Message string
}

func (e *InvalidInputError) Error() string { return e.Message }

func (e *InvalidInputError) Is(target error) bool { return target == ErrInvalidInput }
