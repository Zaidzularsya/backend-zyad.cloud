package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/mail"

	"zyad.cloud/internal/modules/mailbox/domain"
)

// imapOperationTimeout bounds one sync pass for one mailbox (connect,
// search/fetch, and reading every fetched message).
const imapOperationTimeout = 90 * time.Second

// dialIMAP is the real IMAPDialer, used everywhere except tests. It always
// requires TLS: SSL is implicit TLS, STARTTLS is required rather than best
// effort (NewStartTLS fails when the server doesn't offer it) — same policy
// as outbound SMTP (mailbox_service.go's smtpConfig).
func dialIMAP(ctx context.Context, cfg IMAPDialConfig, allowPrivateHosts bool) (IMAPConn, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return nil, errors.New("imap host is required")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: publicDialControl(allowPrivateHosts)}
	address := net.JoinHostPort(host, strconv.Itoa(cfg.Port))
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("imap dial: %w", err)
	}
	if err := conn.SetDeadline(time.Now().Add(imapOperationTimeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	var client *imapclient.Client
	switch cfg.Security {
	case domain.SecuritySSL:
		client = imapclient.New(tls.Client(conn, tlsConfig), nil)
	case domain.SecuritySTARTTLS:
		client, err = imapclient.NewStartTLS(conn, &imapclient.Options{TLSConfig: tlsConfig})
	default:
		_ = conn.Close()
		return nil, fmt.Errorf("imap: unsupported security %q", cfg.Security)
	}
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("imap starttls: %w", err)
	}

	if err := client.Login(cfg.Username, cfg.Password).Wait(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("imap login: %w", err)
	}

	selectData, err := client.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("imap select inbox: %w", err)
	}

	return &goIMAPConn{client: client, uidValidity: selectData.UIDValidity}, nil
}

type goIMAPConn struct {
	client      *imapclient.Client
	uidValidity uint32
}

func (c *goIMAPConn) UIDValidity() uint32 { return c.uidValidity }

func (c *goIMAPConn) Close() error { return c.client.Close() }

var fetchOptions = &imap.FetchOptions{
	UID:      true,
	Envelope: true,
	BodySection: []*imap.FetchItemBodySection{
		{}, // the whole raw message: parsed below for text/html body + attachments
	},
}

func (c *goIMAPConn) FetchSince(ctx context.Context, afterUID uint32, cutoff time.Time, limit int) ([]FetchedMessage, error) {
	var uids []imap.UID
	if afterUID == 0 {
		// First sync of this mailbox: bound by date instead of fetching the
		// whole mailbox history.
		searchData, err := c.client.UIDSearch(&imap.SearchCriteria{Since: cutoff}, nil).Wait()
		if err != nil {
			return nil, fmt.Errorf("imap search: %w", err)
		}
		uids = searchData.AllUIDs()
	} else {
		var set imap.UIDSet
		// Stop 0 means "*" (no upper bound): every UID greater than afterUID.
		set.AddRange(imap.UID(afterUID+1), 0)
		searchData, err := c.client.UIDSearch(&imap.SearchCriteria{UID: []imap.UIDSet{set}}, nil).Wait()
		if err != nil {
			return nil, fmt.Errorf("imap search: %w", err)
		}
		uids = searchData.AllUIDs()
	}
	if len(uids) == 0 {
		return nil, nil
	}
	// Oldest first, so the cursor only advances past messages actually
	// stored; newest within the cap when there are more than `limit`.
	sortUIDs(uids)
	if len(uids) > limit {
		uids = uids[len(uids)-limit:]
	}

	fetchCmd := c.client.Fetch(imap.UIDSetNum(uids...), fetchOptions)
	defer fetchCmd.Close()

	messages := make([]FetchedMessage, 0, len(uids))
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data := fetchCmd.Next()
		if data == nil {
			break
		}
		buf, err := data.Collect()
		if err != nil {
			return nil, fmt.Errorf("imap fetch: %w", err)
		}
		messages = append(messages, parseFetchedMessage(buf, c.uidValidity))
	}
	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}
	return messages, nil
}

func sortUIDs(uids []imap.UID) {
	for i := 1; i < len(uids); i++ {
		for j := i; j > 0 && uids[j-1] > uids[j]; j-- {
			uids[j-1], uids[j] = uids[j], uids[j-1]
		}
	}
}

func parseFetchedMessage(buf *imapclient.FetchMessageBuffer, uidValidity uint32) FetchedMessage {
	message := FetchedMessage{UID: uint32(buf.UID)}
	if env := buf.Envelope; env != nil {
		message.Subject = env.Subject
		message.Date = env.Date
		message.MessageID = messageIDHeader(env.MessageID, uidValidity, message.UID)
		if len(env.InReplyTo) > 0 {
			message.InReplyTo = "<" + strings.Join(env.InReplyTo, "> <") + ">"
		}
		if len(env.From) > 0 {
			message.FromAddress = env.From[0].Addr()
			message.FromName = env.From[0].Name
		}
		message.To = addrStrings(env.To)
		message.Cc = addrStrings(env.Cc)
		message.Bcc = addrStrings(env.Bcc)
	}

	raw := buf.FindBodySection(&imap.FetchItemBodySection{})
	body, err := mail.CreateReader(strings.NewReader(string(raw)))
	if err != nil {
		return message
	}
	defer body.Close()
	for {
		part, err := body.NextPart()
		if err != nil {
			break
		}
		content, readErr := io.ReadAll(part.Body)
		if readErr != nil {
			continue
		}
		switch header := part.Header.(type) {
		case *mail.InlineHeader:
			contentType, _, _ := header.ContentType()
			switch contentType {
			case "text/html":
				if message.BodyHTML == "" {
					message.BodyHTML = string(content)
				}
			case "text/plain":
				if message.BodyText == "" {
					message.BodyText = string(content)
				}
			}
		case *mail.AttachmentHeader:
			filename, _ := header.Filename()
			contentType, _, _ := header.ContentType()
			message.Attachments = append(message.Attachments, FetchedAttachment{
				Filename: filename, ContentType: contentType, Content: content,
			})
		}
	}
	return message
}

// messageIDHeader wraps the envelope's bare Message-ID in angle brackets
// (our storage convention, matching outbound), or synthesizes one when the
// message has none — some senders omit it, and the (mailbox, message_id)
// unique constraint would otherwise collide different messages.
func messageIDHeader(bare string, uidValidity, uid uint32) string {
	if bare != "" {
		return "<" + bare + ">"
	}
	return fmt.Sprintf("<generated-%d-%d@imap-sync>", uidValidity, uid)
}

func addrStrings(addresses []imap.Address) []string {
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, address.Addr())
	}
	return out
}
