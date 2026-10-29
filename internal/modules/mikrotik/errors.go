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
)

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
