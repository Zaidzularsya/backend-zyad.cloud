package mikrotik

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-routeros/routeros/v3"
	"github.com/go-routeros/routeros/v3/proto"
)

// binaryClient speaks the classic RouterOS API (TCP 8728, TLS 8729), used for
// RouterOS v6 which has no REST. Connection lifecycle: every call opens its own
// connection, logs in, runs one command and closes it. That makes the client
// stateless, safe against half-dead sockets and leak-free, at the cost of one
// login round trip per call; the isolir workload is low-volume so this is fine.
// The deadline of that single connection is ep.timeout() (or the ctx deadline
// if sooner) and covers dial, TLS handshake, login and the command.
type binaryClient struct {
	ep Endpoint
}

func newBinaryClient(ep Endpoint) *binaryClient { return &binaryClient{ep: ep} }

func (e Endpoint) binaryPort() int {
	if e.Port != 0 {
		return e.Port
	}
	if e.UseTLS {
		return 8729
	}
	return 8728
}

// normalizeAPIPath turns "ppp/secret" or "/ppp/secret/" into "/ppp/secret".
func normalizeAPIPath(path string) string {
	return "/" + strings.Trim(path, "/")
}

// rowWords renders row as API words with the given prefix ("=" for attributes,
// "?" for query words), sorted for deterministic output. Words are
// length-prefixed on the wire so values cannot inject extra words.
func rowWords(prefix string, r Row) []string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	words := make([]string, 0, len(keys))
	for _, k := range keys {
		words = append(words, prefix+k+"="+r[k])
	}
	return words
}

// sentenceToRow converts a !re sentence. The API already names the record id
// ".id" (word "=.id=*1"), the same key REST uses, so no renaming is needed.
func sentenceToRow(sen *proto.Sentence) Row {
	r := make(Row, len(sen.Map))
	for k, v := range sen.Map {
		r[k] = v
	}
	return r
}

func validateRow(r Row) error {
	for k := range r {
		if k == "" || strings.ContainsAny(k, "=\x00") {
			return errors.New("mikrotik: invalid attribute name")
		}
	}
	return nil
}

func (c *binaryClient) scrub(s string) string {
	if c.ep.Password == "" {
		return s
	}
	return strings.ReplaceAll(s, c.ep.Password, "***")
}

// deviceMessage returns the router's !trap/!fatal text, scrubbed of the
// password before truncation (same rule as REST) and cut on a rune boundary.
func (c *binaryClient) deviceMessage(de *routeros.DeviceError) string {
	msg := de.Sentence.Map["message"]
	if msg == "" && len(de.Sentence.List) > 0 {
		msg = de.Sentence.List[0].Key
	}
	msg = c.scrub(strings.TrimSpace(msg))
	if len(msg) > 200 {
		cut := 200
		for cut > 0 && !utf8.RuneStart(msg[cut]) {
			cut--
		}
		msg = msg[:cut]
	}
	return msg
}

// dial opens the TCP (and optionally TLS) connection with the SSRF guard on
// the dialer and the pin/verify logic shared with REST.
func (c *binaryClient) dial(ctx context.Context) (net.Conn, error) {
	d := &net.Dialer{Control: DialControl(c.ep.AllowedPrivate), Timeout: c.ep.timeout()}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(c.ep.Host, strconv.Itoa(c.ep.binaryPort())))
	if err != nil {
		return nil, err
	}
	if !c.ep.UseTLS {
		return conn, nil
	}
	cfg := tlsConfig(c.ep.TLSFingerprint)
	cfg.ServerName = c.ep.Host
	tc := tls.Client(conn, cfg)
	// The handshake runs in run(), after the connection deadline is set.
	return tc, nil
}

// run opens a connection, logs in, executes words and always closes.
func (c *binaryClient) run(ctx context.Context, words ...string) (*routeros.Reply, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, classifyNetError(err)
	}
	defer conn.Close()

	deadline := time.Now().Add(c.ep.timeout())
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	// The library's sync mode ignores ctx while blocked on a read; closing the
	// connection on cancellation unblocks it.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	var lc *limitedConn
	fail := func(err error) (*routeros.Reply, error) {
		if lc != nil && lc.failure != nil {
			return nil, lc.failure
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, context.Canceled
		}
		return nil, classifyNetError(err)
	}

	if tc, ok := conn.(*tls.Conn); ok {
		if err := tc.HandshakeContext(ctx); err != nil {
			return fail(err)
		}
	}

	lc = &limitedConn{Conn: conn}
	cl, err := routeros.NewClient(lc)
	if err != nil {
		return nil, errors.New("mikrotik: api client setup failed")
	}
	cl.SetLogHandler(slog.DiscardHandler) // never log sentences (login carries the password)

	if err := cl.LoginContext(ctx, c.ep.Username, c.ep.Password); err != nil {
		var de *routeros.DeviceError
		if errors.As(err, &de) {
			return nil, ErrAuthFailed
		}
		return fail(err)
	}
	reply, err := cl.RunArgsContext(ctx, words)
	if err != nil {
		var de *routeros.DeviceError
		var ue *routeros.UnknownReplyError
		switch {
		case errors.As(err, &de):
			return nil, errors.New("mikrotik: router error: " + c.deviceMessage(de))
		case errors.As(err, &ue):
			return nil, errors.New("mikrotik: unexpected response from router")
		}
		return fail(err)
	}
	return reply, nil
}

func (c *binaryClient) Version(ctx context.Context) (string, error) {
	reply, err := c.run(ctx, "/system/resource/print")
	if err != nil {
		return "", err
	}
	if len(reply.Re) == 0 || reply.Re[0].Map["version"] == "" {
		return "", errors.New("mikrotik: router did not report a version")
	}
	return reply.Re[0].Map["version"], nil
}

func (c *binaryClient) Print(ctx context.Context, path string, query Row) ([]Row, error) {
	if err := validateRow(query); err != nil {
		return nil, err
	}
	reply, err := c.run(ctx, append([]string{normalizeAPIPath(path) + "/print"}, rowWords("?", query)...)...)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(reply.Re))
	for _, sen := range reply.Re {
		rows = append(rows, sentenceToRow(sen))
	}
	return rows, nil
}

func (c *binaryClient) Add(ctx context.Context, path string, attrs Row) (string, error) {
	if err := validateRow(attrs); err != nil {
		return "", err
	}
	reply, err := c.run(ctx, append([]string{normalizeAPIPath(path) + "/add"}, rowWords("=", attrs)...)...)
	if err != nil {
		return "", err
	}
	id := ""
	if reply.Done != nil {
		id = reply.Done.Map["ret"]
	}
	if id == "" {
		return "", errors.New("mikrotik: router did not return an id")
	}
	return id, nil
}

func (c *binaryClient) Set(ctx context.Context, path, id string, attrs Row) error {
	if err := validateRow(attrs); err != nil {
		return err
	}
	words := append([]string{normalizeAPIPath(path) + "/set", "=.id=" + id}, rowWords("=", attrs)...)
	_, err := c.run(ctx, words...)
	return err
}

func (c *binaryClient) Remove(ctx context.Context, path, id string) error {
	_, err := c.run(ctx, normalizeAPIPath(path)+"/remove", "=.id="+id)
	return err
}
