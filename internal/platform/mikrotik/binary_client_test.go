package mikrotik

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5" //nolint:gosec // RouterOS pre-6.43 login challenge
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-routeros/routeros/v3/proto"
)

// ---- pure conversion ----

func TestToRowStringifiesAndRenamesID(t *testing.T) {
	sen := proto.NewSentence()
	sen.Word = "!re"
	for _, p := range []proto.Pair{{Key: ".id", Value: "*1"}, {Key: "name", Value: "default"}, {Key: "comment", Value: ""}} {
		sen.List = append(sen.List, p)
		sen.Map[p.Key] = p.Value
	}
	got := sentenceToRow(sen)
	want := Row{".id": "*1", "name": "default", "comment": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestFromRowBuildsSentence(t *testing.T) {
	r := Row{".id": "*2", "name": "a=b", "comment": ""}
	attrs := rowWords("=", r)
	sort.Strings(attrs)
	if want := []string{"=.id=*2", "=comment=", "=name=a=b"}; !reflect.DeepEqual(attrs, want) {
		t.Fatalf("attrs %#v", attrs)
	}
	q := rowWords("?", Row{"name": "x"})
	if !reflect.DeepEqual(q, []string{"?name=x"}) {
		t.Fatalf("query %#v", q)
	}
	if got := normalizeAPIPath("ppp/secret"); got != "/ppp/secret" {
		t.Fatal(got)
	}
	if got := normalizeAPIPath("/ppp/secret/"); got != "/ppp/secret" {
		t.Fatal(got)
	}
}

func TestBinaryRejectsMalformedKeys(t *testing.T) {
	c := newBinaryClient(Endpoint{Host: "127.0.0.1", Port: 1, Timeout: time.Second})
	if _, err := c.Add(context.Background(), "ppp/profile", Row{"a=b": "x"}); err == nil || errors.Is(err, ErrUnreachable) {
		t.Fatalf("want validation error before dialing, got %v", err)
	}
}

func TestBinaryDefaultPorts(t *testing.T) {
	if (Endpoint{}).binaryPort() != 8728 || (Endpoint{UseTLS: true}).binaryPort() != 8729 || (Endpoint{Port: 9, UseTLS: true}).binaryPort() != 9 {
		t.Fatal("unexpected default ports")
	}
}

// ---- in-process fake RouterOS API server ----

type fakeROS struct {
	t         *testing.T
	ln        net.Listener
	user      string
	pass      string
	silent    atomic.Bool // accept, never answer
	trapEcho  atomic.Bool // add traps with the password echoed in the message
	challenge atomic.Bool // pre-6.43 two-step login

	mu     sync.Mutex
	tables map[string][]Row
	next   int
	seen   [][]string // sentences received after login
}

func newFakeROS(t *testing.T, wrap func(net.Listener) net.Listener) *fakeROS {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if wrap != nil {
		ln = wrap(ln)
	}
	f := &fakeROS{t: t, ln: ln, user: "admin", pass: testPassword, tables: map[string][]Row{}}
	t.Cleanup(func() { _ = ln.Close() })
	go f.serve()
	return f
}

func (f *fakeROS) endpoint() Endpoint {
	_, p, _ := net.SplitHostPort(f.ln.Addr().String())
	port, _ := strconv.Atoi(p)
	return Endpoint{Host: "127.0.0.1", Port: port, Username: f.user, Password: f.pass,
		AllowedPrivate: loopbackAllow, Timeout: 2 * time.Second}
}

func (f *fakeROS) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(c)
	}
}

func writeWord(w io.Writer, s string) {
	n := len(s)
	var h []byte
	switch {
	case n < 0x80:
		h = []byte{byte(n)}
	case n < 0x4000:
		h = []byte{byte(n>>8) | 0x80, byte(n)}
	default:
		h = []byte{byte(n>>16) | 0xC0, byte(n >> 8), byte(n)}
	}
	_, _ = w.Write(append(h, s...))
}

func writeSentence(w io.Writer, words ...string) {
	for _, s := range words {
		writeWord(w, s)
	}
	writeWord(w, "")
}

func readWord(r io.Reader) (string, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	n := int(b[0])
	switch {
	case n&0xC0 == 0x80:
		x := make([]byte, 1)
		if _, err := io.ReadFull(r, x); err != nil {
			return "", err
		}
		n = (n&0x3F)<<8 | int(x[0])
	case n&0xE0 == 0xC0:
		x := make([]byte, 2)
		if _, err := io.ReadFull(r, x); err != nil {
			return "", err
		}
		n = (n&0x1F)<<16 | int(x[0])<<8 | int(x[1])
	case n >= 0x80:
		return "", errors.New("fake: unsupported length")
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	return string(buf), err
}

func readSentence(r io.Reader) ([]string, error) {
	var out []string
	for {
		w, err := readWord(r)
		if err != nil {
			return nil, err
		}
		if w == "" {
			return out, nil
		}
		out = append(out, w)
	}
}

func attrsOf(words []string, prefix string) Row {
	r := Row{}
	for _, w := range words {
		if strings.HasPrefix(w, prefix) {
			kv := strings.SplitN(w[1:], "=", 2)
			v := ""
			if len(kv) == 2 {
				v = kv[1]
			}
			r[kv[0]] = v
		}
	}
	return r
}

func (f *fakeROS) handle(c net.Conn) {
	defer c.Close()
	if f.silent.Load() {
		_, _ = io.Copy(io.Discard, c)
		return
	}
	cha := []byte("0123456789abcdef")
	loggedIn := false
	br := bufio.NewReader(c)
	// An HTTP request (REST probe on the same port) is not API protocol:
	// drop the connection like a router with the www service disabled.
	if b, err := br.Peek(1); err != nil || b[0] == 'G' {
		return
	}
	for {
		words, err := readSentence(br)
		if err != nil {
			return
		}
		cmd := words[0]
		if cmd == "/login" {
			a := attrsOf(words[1:], "=")
			switch {
			case f.challenge.Load() && a["response"] == "":
				writeSentence(c, "!done", "=ret="+hex.EncodeToString(cha))
			case f.challenge.Load():
				h := md5.New() //nolint:gosec
				h.Write([]byte{0})
				h.Write([]byte(f.pass))
				h.Write(cha)
				if a["name"] == f.user && a["response"] == "00"+hex.EncodeToString(h.Sum(nil)) {
					loggedIn = true
					writeSentence(c, "!done")
				} else {
					writeSentence(c, "!trap", "=message=cannot log in")
					writeSentence(c, "!done")
				}
			case a["name"] == f.user && a["password"] == f.pass:
				loggedIn = true
				writeSentence(c, "!done")
			default:
				writeSentence(c, "!trap", "=message=invalid user name or password (6)")
				writeSentence(c, "!done")
			}
			continue
		}
		if !loggedIn {
			writeSentence(c, "!fatal", "not logged in")
			return
		}
		f.mu.Lock()
		f.seen = append(f.seen, words)
		f.mu.Unlock()
		f.command(c, words)
	}
}

func (f *fakeROS) command(c net.Conn, words []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := words[0]
	i := strings.LastIndex(cmd, "/")
	path, verb := cmd[:i], cmd[i+1:]
	switch {
	case cmd == "/system/resource/print":
		writeSentence(c, "!re", "=version=6.48.6 (long-term)", "=uptime=1d")
		writeSentence(c, "!done")
	case verb == "print":
		q := attrsOf(words[1:], "?")
		for _, row := range f.tables[path] {
			match := true
			for k, v := range q {
				match = match && row[k] == v
			}
			if match {
				ws := []string{"!re"}
				for k, v := range row {
					ws = append(ws, "="+k+"="+v)
				}
				writeSentence(c, ws...)
			}
		}
		writeSentence(c, "!done")
	case verb == "add":
		if f.trapEcho.Load() {
			writeSentence(c, "!trap", "=message=failure: bad value "+f.pass+" for password")
			writeSentence(c, "!done")
			return
		}
		f.next++
		row := attrsOf(words[1:], "=")
		row[".id"] = fmt.Sprintf("*%X", f.next)
		f.tables[path] = append(f.tables[path], row)
		writeSentence(c, "!done", "=ret="+row[".id"])
	case verb == "set" || verb == "remove":
		a := attrsOf(words[1:], "=")
		for n, row := range f.tables[path] {
			if row[".id"] != a[".id"] {
				continue
			}
			if verb == "remove" {
				f.tables[path] = append(f.tables[path][:n], f.tables[path][n+1:]...)
			} else {
				for k, v := range a {
					row[k] = v
				}
			}
			writeSentence(c, "!done")
			return
		}
		writeSentence(c, "!trap", "=message=no such item")
		writeSentence(c, "!done")
	default:
		writeSentence(c, "!trap", "=message=no such command")
		writeSentence(c, "!done")
	}
}

func newBin(t *testing.T, ep Endpoint) RouterClient {
	t.Helper()
	c, err := NewClient("binary", ep)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestBinaryVersion(t *testing.T) {
	f := newFakeROS(t, nil)
	v, err := newBin(t, f.endpoint()).Version(context.Background())
	if err != nil || v != "6.48.6 (long-term)" {
		t.Fatalf("got %q %v", v, err)
	}
}

func TestBinaryLegacyChallengeLogin(t *testing.T) {
	f := newFakeROS(t, nil)
	f.challenge.Store(true)
	if v, err := newBin(t, f.endpoint()).Version(context.Background()); err != nil || v == "" {
		t.Fatalf("got %q %v", v, err)
	}
	ep := f.endpoint()
	ep.Password = "wrong"
	if _, err := newBin(t, ep).Version(context.Background()); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("want ErrAuthFailed, got %v", err)
	}
}

func TestBinaryAddSetRemovePrint(t *testing.T) {
	f := newFakeROS(t, nil)
	c := newBin(t, f.endpoint())
	ctx := context.Background()

	id, err := c.Add(ctx, "ppp/profile", Row{"name": "zyad-isolir", "rate-limit": "64k/64k"})
	if err != nil || id != "*1" {
		t.Fatalf("add: %q %v", id, err)
	}
	if err := c.Set(ctx, "/ppp/profile", id, Row{"rate-limit": "128k/128k"}); err != nil {
		t.Fatal(err)
	}
	rows, err := c.Print(ctx, "ppp/profile", Row{"name": "zyad-isolir"})
	if err != nil || len(rows) != 1 || rows[0][".id"] != id || rows[0]["rate-limit"] != "128k/128k" {
		t.Fatalf("print: %#v %v", rows, err)
	}
	if rows, err := c.Print(ctx, "ppp/profile", Row{"name": "other"}); err != nil || len(rows) != 0 {
		t.Fatalf("filtered print: %#v %v", rows, err)
	}
	if err := c.Remove(ctx, "ppp/profile", id); err != nil {
		t.Fatal(err)
	}
	rows, _ = c.Print(ctx, "ppp/profile", nil)
	if len(rows) != 0 {
		t.Fatalf("not removed: %#v", rows)
	}
	// set on a missing id surfaces the router message
	if err := c.Remove(ctx, "ppp/profile", "*99"); err == nil || !strings.Contains(err.Error(), "no such item") {
		t.Fatalf("want router message, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := f.seen[0][0]; got != "/ppp/profile/add" {
		t.Fatalf("command word %q", got)
	}
}

func TestBinaryAuthFailed(t *testing.T) {
	f := newFakeROS(t, nil)
	ep := f.endpoint()
	ep.Password = "not-the-password"
	_, err := newBin(t, ep).Version(context.Background())
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("want ErrAuthFailed, got %v", err)
	}
	if strings.Contains(err.Error(), "not-the-password") {
		t.Fatal("password leaked")
	}
}

func TestBinaryTrapMessageScrubbed(t *testing.T) {
	f := newFakeROS(t, nil)
	f.trapEcho.Store(true)
	_, err := newBin(t, f.endpoint()).Add(context.Background(), "ppp/secret", Row{"name": "x"})
	if err == nil || strings.Contains(err.Error(), testPassword) || !strings.Contains(err.Error(), "***") {
		t.Fatalf("want scrubbed trap error, got %v", err)
	}
	if errors.Is(err, ErrUnreachable) || errors.Is(err, ErrAuthFailed) {
		t.Fatalf("trap must not map to a sentinel: %v", err)
	}
}

func TestBinaryTimeout(t *testing.T) {
	f := newFakeROS(t, nil)
	f.silent.Store(true)
	ep := f.endpoint()
	ep.Timeout = 150 * time.Millisecond
	start := time.Now()
	_, err := newBin(t, ep).Version(context.Background())
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("want ErrUnreachable, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
}

func TestBinaryContextCancel(t *testing.T) {
	f := newFakeROS(t, nil)
	f.silent.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	_, err := newBin(t, f.endpoint()).Version(ctx)
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("got %v after %v", err, time.Since(start))
	}
}

func TestBinaryUnreachableAndHostGuard(t *testing.T) {
	ep := Endpoint{Host: "127.0.0.1", Port: 1, Username: "a", Password: testPassword, AllowedPrivate: loopbackAllow, Timeout: time.Second}
	if _, err := newBin(t, ep).Version(context.Background()); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("refused: %v", err)
	}
	f := newFakeROS(t, nil)
	ep = f.endpoint()
	ep.AllowedPrivate = nil
	_, err := newBin(t, ep).Version(context.Background())
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("want ErrHostNotAllowed, got %v", err)
	}
}

func selfSigned(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "router"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:    x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, hex.EncodeToString(sum[:])
}

func newTLSFake(t *testing.T) (*fakeROS, string) {
	cert, fp := selfSigned(t)
	f := newFakeROS(t, func(l net.Listener) net.Listener {
		return tls.NewListener(l, &tls.Config{Certificates: []tls.Certificate{cert}})
	})
	return f, fp
}

func TestBinaryTLS(t *testing.T) {
	f, fp := newTLSFake(t)
	ep := f.endpoint()
	ep.UseTLS = true

	// untrusted self-signed leaf, no pin
	_, err := newBin(t, ep).Version(context.Background())
	var tu *TLSUntrustedError
	if !errors.Is(err, ErrTLSUntrusted) || !errors.As(err, &tu) || tu.Fingerprint != fp || tu.PinMismatch {
		t.Fatalf("untrusted: %v", err)
	}

	// matching pin (colon form accepted)
	ep.TLSFingerprint = strings.ToUpper(fp[:2]) + ":" + fp[2:]
	if v, err := newBin(t, ep).Version(context.Background()); err != nil || v == "" {
		t.Fatalf("pinned: %q %v", v, err)
	}

	// wrong pin
	ep.TLSFingerprint = strings.Repeat("ab", 32)
	_, err = newBin(t, ep).Version(context.Background())
	if !errors.As(err, &tu) || !tu.PinMismatch || tu.Fingerprint != fp {
		t.Fatalf("mismatch: %v", err)
	}
}

func TestDetectFallsBackToBinary(t *testing.T) {
	f := newFakeROS(t, nil)
	// Detect probes REST first on the same port; the fake drops HTTP requests,
	// which classifies as ErrUnreachable and triggers the binary fallback.
	ep := f.endpoint()
	typ, ver, err := Detect(context.Background(), ep)
	if err != nil || typ != APITypeBinary || ver != "6.48.6 (long-term)" {
		t.Fatalf("got %q %q %v", typ, ver, err)
	}
}
