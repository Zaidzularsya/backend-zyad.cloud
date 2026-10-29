package mikrotik

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testPassword = "s3cr3t-P@ss-do-not-leak"

var loopbackAllow = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}

func endpointFor(t *testing.T, srv *httptest.Server, useTLS bool) Endpoint {
	t.Helper()
	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(strings.TrimPrefix(srv.URL, "https://"), "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	return Endpoint{
		Host: host, Port: port, Username: "admin", Password: testPassword,
		UseTLS: useTLS, AllowedPrivate: loopbackAllow, Timeout: 2 * time.Second,
	}
}

func leafFingerprint(srv *httptest.Server) string {
	sum := sha256.Sum256(srv.Certificate().Raw)
	return hex.EncodeToString(sum[:])
}

func newREST(t *testing.T, ep Endpoint) RouterClient {
	t.Helper()
	c, err := NewClient("rest", ep)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRESTClientPrint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "admin" || p != testPassword {
			t.Errorf("bad basic auth: %v %q", ok, u)
		}
		if r.Method != http.MethodGet || r.URL.Path != "/rest/ppp/profile" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("name") != "default" {
			t.Errorf("query not forwarded: %s", r.URL.RawQuery)
		}
		if strings.Contains(r.URL.String(), testPassword) {
			t.Error("password in URL")
		}
		_, _ = w.Write([]byte(`[{".id":"*1","name":"default","only-one":true,"rate":10,"x":null}]`))
	}))
	defer srv.Close()

	rows, err := newREST(t, endpointFor(t, srv, false)).Print(context.Background(), "ppp/profile", Row{"name": "default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0][".id"] != "*1" || rows[0]["only-one"] != "true" || rows[0]["rate"] != "10" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

func TestRESTClientVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/system/resource" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"version":"7.14.1 (stable)"}`))
	}))
	defer srv.Close()
	v, err := newREST(t, endpointFor(t, srv, false)).Version(context.Background())
	if err != nil || v != "7.14.1 (stable)" {
		t.Fatalf("got %q %v", v, err)
	}
}

func TestRESTClientAddSetRemove(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		switch r.Method {
		case http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "isolir" {
				t.Errorf("body %v", body)
			}
			_, _ = w.Write([]byte(`{".id":"*A","name":"isolir"}`))
		case http.MethodPatch:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["profile"] != "isolir" {
				t.Errorf("body %v", body)
			}
			_, _ = w.Write([]byte(`{".id":"*A"}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	c := newREST(t, endpointFor(t, srv, false))
	ctx := context.Background()
	id, err := c.Add(ctx, "ppp/profile", Row{"name": "isolir"})
	if err != nil || id != "*A" {
		t.Fatalf("add: %q %v", id, err)
	}
	if err := c.Set(ctx, "ppp/secret", id, Row{"profile": "isolir"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(ctx, "ppp/secret", id); err != nil {
		t.Fatal(err)
	}
	want := []string{"PUT /rest/ppp/profile", "PATCH /rest/ppp/secret/*A", "DELETE /rest/ppp/secret/*A"}
	if strings.Join(seen, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v want %v", seen, want)
	}
}

func TestRESTClientAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := newREST(t, endpointFor(t, srv, false)).Version(context.Background())
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("want ErrAuthFailed, got %v", err)
	}
}

func TestRESTClientTLSUntrusted(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"7.1"}`))
	}))
	defer srv.Close()
	_, err := newREST(t, endpointFor(t, srv, true)).Version(context.Background())
	var tu *TLSUntrustedError
	if !errors.As(err, &tu) || !errors.Is(err, ErrTLSUntrusted) {
		t.Fatalf("want TLSUntrustedError, got %v", err)
	}
	if tu.Fingerprint != leafFingerprint(srv) {
		t.Fatalf("fingerprint %s want %s", tu.Fingerprint, leafFingerprint(srv))
	}
}

func TestRESTClientPinnedFingerprint(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"7.1"}`))
	}))
	defer srv.Close()

	ep := endpointFor(t, srv, true)
	ep.TLSFingerprint = strings.ToUpper(leafFingerprint(srv))
	if v, err := newREST(t, ep).Version(context.Background()); err != nil || v != "7.1" {
		t.Fatalf("pinned: %q %v", v, err)
	}

	ep.TLSFingerprint = strings.Repeat("ab", 32)
	_, err := newREST(t, ep).Version(context.Background())
	if !errors.Is(err, ErrTLSUntrusted) {
		t.Fatalf("mismatch must be rejected as ErrTLSUntrusted, got %v", err)
	}
}

func TestRESTClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
	}))
	defer srv.Close()
	ep := endpointFor(t, srv, false)
	ep.Timeout = 100 * time.Millisecond
	start := time.Now()
	_, err := newREST(t, ep).Version(context.Background())
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("want ErrUnreachable, got %v", err)
	}
	if time.Since(start) > 350*time.Millisecond {
		t.Fatalf("timeout not honored: %v", time.Since(start))
	}
}

func TestRESTClientHostNotAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not reach the server")
	}))
	defer srv.Close()
	ep := endpointFor(t, srv, false)
	ep.AllowedPrivate = nil
	_, err := newREST(t, ep).Version(context.Background())
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("want ErrHostNotAllowed, got %v", err)
	}
}

func TestRESTClientIgnoresProxyEnv(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("http_proxy", "http://127.0.0.1:1")
	t.Setenv("https_proxy", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"7.1"}`))
	}))
	defer srv.Close()
	if _, err := newREST(t, endpointFor(t, srv, false)).Version(context.Background()); err != nil {
		t.Fatalf("proxy env must be ignored: %v", err)
	}
}

func TestRESTClientErrorNeverContainsPassword(t *testing.T) {
	ctx := context.Background()
	check := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("%s: expected an error", name)
			return
		}
		if strings.Contains(err.Error(), testPassword) {
			t.Errorf("%s: password leaked: %v", name, err)
		}
	}

	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":500,"message":"bad ` + testPassword + `","detail":"` + testPassword + `"}`))
	}))
	defer echo.Close()
	_, err := newREST(t, endpointFor(t, echo, false)).Version(ctx)
	check("500 echo", err)
	_, err = newREST(t, endpointFor(t, echo, false)).Print(ctx, "ppp/secret", nil)
	check("500 print", err)

	unauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauth.Close()
	_, err = newREST(t, endpointFor(t, unauth, false)).Version(ctx)
	check("401", err)

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer tlsSrv.Close()
	_, err = newREST(t, endpointFor(t, tlsSrv, true)).Version(ctx)
	check("untrusted", err)
	ep := endpointFor(t, tlsSrv, true)
	ep.TLSFingerprint = strings.Repeat("cd", 32)
	_, err = newREST(t, ep).Version(ctx)
	check("pin mismatch", err)

	ep = endpointFor(t, echo, false)
	ep.AllowedPrivate = nil
	_, err = newREST(t, ep).Version(ctx)
	check("not allowed", err)

	ep = endpointFor(t, echo, false)
	ep.Port = 1 // nothing listens here
	_, err = newREST(t, ep).Version(ctx)
	check("refused", err)

	ep = endpointFor(t, echo, false)
	ep.Host = "no-such-host.invalid"
	_, err = newREST(t, ep).Version(ctx)
	check("dns", err)
}

func TestNewClientBinaryNotYet(t *testing.T) {
	if _, err := NewClient("binary", Endpoint{Host: "x"}); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("want not-implemented error, got %v", err)
	}
}

func TestNewClientUnknownType(t *testing.T) {
	if _, err := NewClient("soap", Endpoint{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDetectREST(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"7.12"}`))
	}))
	defer srv.Close()
	typ, ver, err := Detect(context.Background(), endpointFor(t, srv, false))
	if err != nil || typ != "rest" || ver != "7.12" {
		t.Fatalf("got %q %q %v", typ, ver, err)
	}
}

func TestDetectDoesNotFallBackOnAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, _, err := Detect(context.Background(), endpointFor(t, srv, false))
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("want ErrAuthFailed, got %v", err)
	}
}

func TestDetectUnreachable(t *testing.T) {
	ep := Endpoint{Host: "127.0.0.1", Port: 1, AllowedPrivate: loopbackAllow, Timeout: time.Second}
	_, _, err := Detect(context.Background(), ep)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("want ErrUnreachable, got %v", err)
	}
}

func TestFakeClient(t *testing.T) {
	f := NewFakeClient()
	ctx := context.Background()
	id, err := f.Add(ctx, "ppp/secret", Row{"name": "a"})
	if err != nil || id == "" {
		t.Fatal(id, err)
	}
	if err := f.Set(ctx, "ppp/secret", id, Row{"profile": "x"}); err != nil {
		t.Fatal(err)
	}
	rows, _ := f.Print(ctx, "ppp/secret", Row{"name": "a"})
	if len(rows) != 1 || rows[0]["profile"] != "x" {
		t.Fatalf("%#v", rows)
	}
	if err := f.Remove(ctx, "ppp/secret", id); err != nil {
		t.Fatal(err)
	}
	if f.WriteCalls() != 3 {
		t.Fatalf("writes %d", f.WriteCalls())
	}
	f.Err = ErrUnreachable
	if _, err := f.Version(ctx); !errors.Is(err, ErrUnreachable) {
		t.Fatal(err)
	}
}

var _ RouterClient = (*FakeClient)(nil)

func TestRESTClientRefusesRedirects(t *testing.T) {
	var hit bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = w.Write([]byte(`{"version":"7.1"}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/rest/system/resource", http.StatusFound)
	}))
	defer srv.Close()
	_, err := newREST(t, endpointFor(t, srv, false)).Version(context.Background())
	if err == nil {
		t.Fatal("expected an error for a redirect")
	}
	if hit {
		t.Fatal("redirect target must never be contacted")
	}
	if strings.Contains(err.Error(), testPassword) {
		t.Fatalf("password leaked: %v", err)
	}
}

func TestRESTClientScrubsPasswordAcrossTruncation(t *testing.T) {
	const pw = "Zq9!xKw#7mPv"
	msg := strings.Repeat("a", 195) + pw + strings.Repeat("b", 50)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		b, _ := json.Marshal(map[string]string{"message": msg})
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	ep := endpointFor(t, srv, false)
	ep.Password = pw
	_, err := newREST(t, ep).Version(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	for n := 3; n <= len(pw); n++ {
		if strings.Contains(err.Error(), pw[:n]) {
			t.Fatalf("password prefix %q leaked: %v", pw[:n], err)
		}
	}
}
