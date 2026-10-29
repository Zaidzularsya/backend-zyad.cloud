package mikrotik

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"
)

// DefaultTimeout bounds one router request when Endpoint.Timeout is zero.
const DefaultTimeout = 10 * time.Second

// API types accepted by NewClient.
const (
	APITypeREST   = "rest"
	APITypeBinary = "binary"
)

var (
	// ErrUnreachable: DNS failure, connection refused, or timeout.
	ErrUnreachable = errors.New("mikrotik: router unreachable")
	// ErrAuthFailed: the router rejected the credentials.
	ErrAuthFailed = errors.New("mikrotik: authentication failed")
	// ErrNotFound is returned by FakeClient when an id does not exist.
	ErrNotFound = errors.New("mikrotik: record not found")
	// ErrTLSUntrusted matches TLSUntrustedError (unknown certificate or pin mismatch).
	ErrTLSUntrusted = errors.New("mikrotik: tls certificate not trusted")
)

// TLSUntrustedError reports a certificate that failed verification (or did not
// match the pin). Fingerprint is the leaf certificate SHA-256 as lowercase hex
// so a UI can offer to pin it. Pin mismatches use the same type, with the
// fingerprint actually presented by the router.
type TLSUntrustedError struct {
	Fingerprint string
	// PinMismatch is true when a pin was configured and did not match.
	PinMismatch bool
}

func (e *TLSUntrustedError) Error() string {
	if e.PinMismatch {
		return "mikrotik: tls certificate does not match pinned fingerprint (presented sha256 " + e.Fingerprint + ")"
	}
	return "mikrotik: tls certificate not trusted (sha256 " + e.Fingerprint + ")"
}

func (e *TLSUntrustedError) Is(target error) bool { return target == ErrTLSUntrusted }

// Row is one RouterOS record; all values are strings.
type Row map[string]string

// Endpoint describes how to reach one router.
type Endpoint struct {
	Host     string
	Port     int // 0 = 443 when UseTLS, else 80
	Username string
	Password string
	UseTLS   bool
	// TLSFingerprint pins the leaf certificate (SHA-256 hex, colons allowed).
	// Empty means normal system verification.
	TLSFingerprint string
	// AllowedPrivate lists non-public ranges the host guard may connect to.
	AllowedPrivate []netip.Prefix
	// Timeout per request; zero means DefaultTimeout.
	Timeout time.Duration
}

func (e Endpoint) timeout() time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return DefaultTimeout
}

func (e Endpoint) port() int {
	if e.Port != 0 {
		return e.Port
	}
	if e.UseTLS {
		return 443
	}
	return 80
}

// RouterClient is the minimal RouterOS surface the isolir feature needs.
type RouterClient interface {
	Version(ctx context.Context) (string, error)
	Print(ctx context.Context, path string, query Row) ([]Row, error)
	Add(ctx context.Context, path string, attrs Row) (id string, err error)
	Set(ctx context.Context, path, id string, attrs Row) error
	Remove(ctx context.Context, path, id string) error
}

// NewClient builds a client for apiType ("rest" or "binary").
func NewClient(apiType string, ep Endpoint) (RouterClient, error) {
	switch apiType {
	case APITypeREST:
		return newRESTClient(ep), nil
	case APITypeBinary:
		return newBinaryClient(ep), nil
	default:
		return nil, fmt.Errorf("mikrotik: unknown api type %q", apiType)
	}
}

// detectFirstProbeMax bounds the first probe when a second one can follow, so
// a service that swallows the probe silently (an API port receiving HTTP waits
// for a terminator and never answers) cannot eat the whole budget.
const detectFirstProbeMax = 5 * time.Second

// Detect probes the router with both transports and returns the one that
// works. Order: REST first, except when ep.Port is 8728 or 8729 (the API
// service ports), where binary goes first. The first probe gets
// min(ep.Timeout, 5s); the second gets the endpoint timeout capped by what is
// left of ctx, so callers should allow roughly 2x the endpoint timeout in ctx.
//
// Any failure of the first probe falls back to the other transport, except
// ErrAuthFailed, TLS trust errors and ErrHostNotAllowed (another transport
// would not help) and context cancellation, which are returned immediately.
// If both fail: the second error when it is auth/TLS trust/host-not-allowed,
// otherwise ErrUnreachable (fixed text, never raw router or library text).
func Detect(ctx context.Context, ep Endpoint) (apiType, version string, err error) {
	order := detectOrder(ep)

	first := ep
	if t := ep.timeout(); t > detectFirstProbeMax {
		first.Timeout = detectFirstProbeMax
	}
	version, err = probe(ctx, order[0], first)
	if err == nil {
		return order[0], version, nil
	}
	if detectFatal(ctx, err) {
		return "", "", err
	}

	second := ep
	if dl, ok := ctx.Deadline(); ok {
		left := time.Until(dl)
		if left <= 0 {
			return "", "", ErrUnreachable
		}
		if left < second.timeout() {
			second.Timeout = left
		}
	}
	version, err = probe(ctx, order[1], second)
	if err == nil {
		return order[1], version, nil
	}
	if detectFatal(ctx, err) {
		return "", "", err
	}
	return "", "", ErrUnreachable
}

// detectOrder puts binary first on the API service ports, REST otherwise.
func detectOrder(ep Endpoint) [2]string {
	if ep.Port == 8728 || ep.Port == 8729 {
		return [2]string{APITypeBinary, APITypeREST}
	}
	return [2]string{APITypeREST, APITypeBinary}
}

// detectFatal reports errors that another transport cannot cure.
func detectFatal(ctx context.Context, err error) bool {
	return ctx.Err() != nil ||
		errors.Is(err, ErrAuthFailed) ||
		errors.Is(err, ErrTLSUntrusted) ||
		errors.Is(err, ErrHostNotAllowed) ||
		errors.Is(err, context.Canceled)
}

func probe(ctx context.Context, apiType string, ep Endpoint) (string, error) {
	c, err := NewClient(apiType, ep)
	if err != nil {
		return "", err
	}
	return c.Version(ctx)
}
