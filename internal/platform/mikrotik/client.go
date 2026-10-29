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

// Detect probes the router: REST first, then the binary API. Auth and TLS
// trust errors are returned immediately (another transport would not help).
func Detect(ctx context.Context, ep Endpoint) (apiType, version string, err error) {
	rest, err := NewClient(APITypeREST, ep)
	if err != nil {
		return "", "", err
	}
	version, restErr := rest.Version(ctx)
	if restErr == nil {
		return APITypeREST, version, nil
	}
	if !errors.Is(restErr, ErrUnreachable) {
		return "", "", restErr
	}
	bin, err := NewClient(APITypeBinary, ep)
	if err != nil {
		return "", "", restErr // binary unavailable: report the REST failure
	}
	if version, err = bin.Version(ctx); err != nil {
		return "", "", err
	}
	return APITypeBinary, version, nil
}
