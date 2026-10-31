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

// ErrSetupInProgress: another Apply or Cleanup holds this router's setup lock;
// nothing was read or written.
var ErrSetupInProgress = errors.New("another setup apply or cleanup is running for this router; try again when it finishes")

// ErrSetupRESTUnverified: setup apply/cleanup refused for a router on the REST
// transport while MIKROTIK_SETUP_ALLOW_REST is off; nothing was written.
var ErrSetupRESTUnverified = errors.New("setup over the REST transport has not been verified on a real router yet; use the binary API (port 8728/8729) or enable MIKROTIK_SETUP_ALLOW_REST")

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

// Fase 2: pemetaan PPP secret ke pelanggan.
var (
	// ErrAccountNotFound: the receivable account does not exist in this organization.
	ErrAccountNotFound = errors.New("account not found")
	// ErrProductNotFound: the product does not exist in this organization.
	ErrProductNotFound = errors.New("product not found")
	// ErrSecretNotFound: the PPP secret does not exist in this organization.
	ErrSecretNotFound = errors.New("ppp secret not found")
	// ErrSecretNotPairable: the PPP secret cannot be paired to a customer yet.
	ErrSecretNotPairable = errors.New("ppp secret cannot be paired")
	// ErrSecretAlreadyPaired: the PPP secret is already paired to a customer.
	ErrSecretAlreadyPaired = errors.New("ppp secret is already paired")
	// ErrSecretIsolated: the PPP secret is currently isolated and cannot be unpaired.
	ErrSecretIsolated = errors.New("ppp secret is isolated")
)

// Fase 3: mesin isolir.
var (
	// ErrTaskNotFound: the isolation task does not exist in this organization.
	ErrTaskNotFound = errors.New("isolation task not found")
	// ErrTaskNotRetryable: only a failed isolation task can be retried.
	ErrTaskNotRetryable = errors.New("isolation task is not in a retryable state")
	// ErrTaskActiveExists: another pending/running task already exists for the secret.
	ErrTaskActiveExists = errors.New("an active task already exists for this secret")
	// ErrRouterHasIsolatedSecrets: the router still has isolated secrets or
	// secrets with a pending/running isolation task, so it cannot be deleted
	// (nothing could restore those customers afterwards).
	ErrRouterHasIsolatedSecrets = errors.New("router has isolated secrets or active isolation tasks")
	// ErrSecretHasActiveTask: the PPP secret has a pending/running isolation
	// task and cannot be unpaired until it finishes.
	ErrSecretHasActiveTask = errors.New("ppp secret has an active isolation task")
)

// Fase 3: langkah router pada eksekutor isolir.
var (
	// ErrSecretMissingOnRouter: the PPP secret no longer exists on the router.
	ErrSecretMissingOnRouter = errors.New("ppp secret is missing on the router")
	// ErrRestoreProfileMissing: the profile to restore to does not exist on the router.
	ErrRestoreProfileMissing = errors.New("restore profile does not exist on the router")
	// ErrRestoreTargetInvalid: no usable original profile is known (empty or the isolir profile).
	ErrRestoreTargetInvalid = errors.New("no valid original profile to restore to")
	// ErrRouterNotIsolirReady: the router has no completed isolir setup.
	ErrRouterNotIsolirReady = errors.New("router is not isolir ready")
)
