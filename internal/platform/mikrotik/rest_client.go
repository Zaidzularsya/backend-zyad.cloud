package mikrotik

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

type restClient struct {
	ep   Endpoint
	http *http.Client
}

func newRESTClient(ep Endpoint) *restClient {
	dialer := &net.Dialer{Timeout: ep.timeout(), Control: DialControl(ep.AllowedPrivate)}
	tr := &http.Transport{
		// Never honor *_PROXY: DialControl must see the router address itself.
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		TLSClientConfig:     tlsConfig(ep.TLSFingerprint),
		TLSHandshakeTimeout: ep.timeout(),
		DisableKeepAlives:   true,
	}
	return &restClient{ep: ep, http: &http.Client{
		Transport: tr,
		Timeout:   ep.timeout(),
		// RouterOS REST never redirects; following one would let a
		// tenant-controlled router steer requests (and credentials) elsewhere.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func normalizeFingerprint(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), ":", ""))
}

// tlsConfig never disables verification: the built-in check is replaced by
// VerifyConnection, which either compares the pinned leaf hash (pin replaces
// chain validation) or performs full system verification itself.
func tlsConfig(pin string) *tls.Config {
	pin = normalizeFingerprint(pin)
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // replaced by VerifyConnection below
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no peer certificate")
			}
			leaf := cs.PeerCertificates[0]
			sum := sha256.Sum256(leaf.Raw)
			fp := hex.EncodeToString(sum[:])
			if pin != "" {
				if fp != pin {
					return &TLSUntrustedError{Fingerprint: fp, PinMismatch: true}
				}
				return nil
			}
			inter := x509.NewCertPool()
			for _, c := range cs.PeerCertificates[1:] {
				inter.AddCert(c)
			}
			if _, err := leaf.Verify(x509.VerifyOptions{DNSName: cs.ServerName, Intermediates: inter}); err != nil {
				return &TLSUntrustedError{Fingerprint: fp}
			}
			return nil
		},
	}
}

func (c *restClient) baseURL() string {
	scheme := "http"
	if c.ep.UseTLS {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(c.ep.Host, strconv.Itoa(c.ep.port())) + "/rest/"
}

// scrub removes the password from any text before it reaches an error.
func (c *restClient) scrub(s string) string {
	if c.ep.Password == "" {
		return s
	}
	return strings.ReplaceAll(s, c.ep.Password, "***")
}

// do performs one request and returns the body of a 2xx response.
func (c *restClient) do(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	u := c.baseURL() + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("mikrotik: encode request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, errors.New("mikrotik: build request failed")
	}
	req.SetBasicAuth(c.ep.Username, c.ep.Password)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.classify(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, c.classify(err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ErrAuthFailed
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return nil, errors.New(c.scrub(fmt.Sprintf("mikrotik: router returned %d: %s", resp.StatusCode, c.apiMessage(data))))
	}
	return data, nil
}

// apiMessage extracts RouterOS' {"message","detail"} error text. The password
// is scrubbed before truncating so a value echoed across the cut cannot leave
// a prefix behind; truncation happens on a rune boundary.
func (c *restClient) apiMessage(data []byte) string {
	var e struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}
	if json.Unmarshal(data, &e) != nil {
		return ""
	}
	msg := c.scrub(strings.TrimSpace(e.Message + " " + e.Detail))
	if len(msg) > 200 {
		cut := 200
		for cut > 0 && !utf8.RuneStart(msg[cut]) {
			cut--
		}
		msg = msg[:cut]
	}
	return msg
}

// classify maps transport failures onto the package sentinels. The cause text
// is deliberately not included for unreachable errors beyond a fixed reason.
func (c *restClient) classify(err error) error {
	var tu *TLSUntrustedError
	switch {
	case errors.As(err, &tu):
		return tu
	case errors.Is(err, ErrHostNotAllowed):
		return fmt.Errorf("%w", ErrHostNotAllowed)
	case errors.Is(err, context.Canceled):
		return context.Canceled
	}
	reason := "connection failed"
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.As(err, &dnsErr):
		reason = "dns lookup failed"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		reason = "timeout"
	case strings.Contains(err.Error(), "connection refused"):
		reason = "connection refused"
	}
	return fmt.Errorf("%w: %s", ErrUnreachable, reason)
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func toRow(m map[string]any) Row {
	r := make(Row, len(m))
	for k, v := range m {
		r[k] = stringify(v)
	}
	return r
}

func (c *restClient) Version(ctx context.Context) (string, error) {
	data, err := c.do(ctx, http.MethodGet, "system/resource", nil, nil)
	if err != nil {
		return "", err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return "", errors.New("mikrotik: unexpected response from router")
	}
	v := stringify(m["version"])
	if v == "" {
		return "", errors.New("mikrotik: router did not report a version")
	}
	return v, nil
}

func (c *restClient) Print(ctx context.Context, path string, query Row) ([]Row, error) {
	q := url.Values{}
	for k, v := range query {
		q.Set(k, v)
	}
	data, err := c.do(ctx, http.MethodGet, path, q, nil)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, errors.New("mikrotik: unexpected response from router")
	}
	rows := make([]Row, 0, len(list))
	for _, m := range list {
		rows = append(rows, toRow(m))
	}
	return rows, nil
}

func (c *restClient) Add(ctx context.Context, path string, attrs Row) (string, error) {
	data, err := c.do(ctx, http.MethodPut, path, nil, attrs)
	if err != nil {
		return "", err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return "", errors.New("mikrotik: unexpected response from router")
	}
	id := stringify(m[".id"])
	if id == "" {
		return "", errors.New("mikrotik: router did not return an id")
	}
	return id, nil
}

func (c *restClient) Set(ctx context.Context, path, id string, attrs Row) error {
	_, err := c.do(ctx, http.MethodPatch, strings.TrimRight(path, "/")+"/"+url.PathEscape(id), nil, attrs)
	return err
}

func (c *restClient) Remove(ctx context.Context, path, id string) error {
	_, err := c.do(ctx, http.MethodDelete, strings.TrimRight(path, "/")+"/"+url.PathEscape(id), nil, nil)
	return err
}
