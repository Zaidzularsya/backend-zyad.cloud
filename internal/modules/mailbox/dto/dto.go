package dto

import (
	"time"

	"zyad.cloud/internal/modules/mailbox/domain"
)

type MailboxRequest struct {
	EmailAddress string `json:"email_address" binding:"required"`
	DisplayName  string `json:"display_name"`
	Username     string `json:"username"`
	// Password is required on create; omitted on update keeps the old one.
	Password     string `json:"password"`
	SMTPHost     string `json:"smtp_host" binding:"required"`
	SMTPPort     int    `json:"smtp_port" binding:"required"`
	SMTPSecurity string `json:"smtp_security" binding:"required"`
	IMAPHost     string `json:"imap_host"`
	IMAPPort     int    `json:"imap_port"`
	IMAPSecurity string `json:"imap_security"`
}

type MessageListQuery struct {
	Page              int    `form:"page"`
	PerPage           int    `form:"per_page"`
	MailboxID         string `form:"mailbox_id"`
	Participant       string `form:"participant"`
	Direction         string `form:"direction"`
	RelatedEntityType string `form:"related_entity_type"`
	RelatedEntityID   string `form:"related_entity_id"`
}

// MailboxResponse never includes the password.
type MailboxResponse struct {
	ID           string    `json:"id"`
	EmailAddress string    `json:"email_address"`
	DisplayName  string    `json:"display_name"`
	Username     string    `json:"username"`
	SMTPHost     string    `json:"smtp_host"`
	SMTPPort     int       `json:"smtp_port"`
	SMTPSecurity string    `json:"smtp_security"`
	IMAPHost     string    `json:"imap_host"`
	IMAPPort     int       `json:"imap_port"`
	IMAPSecurity string    `json:"imap_security"`
	Status       string    `json:"status"`
	LastError    string    `json:"last_error"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func MailboxFromDomain(m domain.Mailbox) MailboxResponse {
	return MailboxResponse{
		ID: m.ID, EmailAddress: m.EmailAddress, DisplayName: m.DisplayName, Username: m.Username,
		SMTPHost: m.SMTPHost, SMTPPort: m.SMTPPort, SMTPSecurity: string(m.SMTPSecurity),
		IMAPHost: m.IMAPHost, IMAPPort: m.IMAPPort, IMAPSecurity: string(m.IMAPSecurity),
		Status: string(m.Status), LastError: m.LastError, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func MailboxListFromDomain(mailboxes []domain.Mailbox) []MailboxResponse {
	out := make([]MailboxResponse, 0, len(mailboxes))
	for _, m := range mailboxes {
		out = append(out, MailboxFromDomain(m))
	}
	return out
}

type AttachmentResponse struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

type MessageResponse struct {
	ID                string               `json:"id"`
	MailboxID         string               `json:"mailbox_id"`
	Direction         string               `json:"direction"`
	Status            string               `json:"status"`
	MessageID         string               `json:"message_id"`
	InReplyTo         string               `json:"in_reply_to,omitempty"`
	FromAddress       string               `json:"from_address"`
	FromName          string               `json:"from_name,omitempty"`
	To                []string             `json:"to"`
	Cc                []string             `json:"cc"`
	Bcc               []string             `json:"bcc"`
	Subject           string               `json:"subject"`
	Snippet           string               `json:"snippet"`
	BodyHTML          string               `json:"body_html,omitempty"`
	Error             string               `json:"error,omitempty"`
	RelatedEntityType string               `json:"related_entity_type,omitempty"`
	RelatedEntityID   string               `json:"related_entity_id,omitempty"`
	SentAt            *time.Time           `json:"sent_at"`
	CreatedAt         time.Time            `json:"created_at"`
	Attachments       []AttachmentResponse `json:"attachments"`
}

// MessageFromDomain omits the body when withBody is false (list views only
// need the snippet).
func MessageFromDomain(m domain.Message, withBody bool) MessageResponse {
	attachments := make([]AttachmentResponse, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		attachments = append(attachments, AttachmentResponse{ID: a.ID, Filename: a.Filename, MimeType: a.MimeType, SizeBytes: a.SizeBytes})
	}
	response := MessageResponse{
		ID: m.ID, MailboxID: m.MailboxID, Direction: string(m.Direction), Status: string(m.Status),
		MessageID: m.MessageID, InReplyTo: m.InReplyTo, FromAddress: m.FromAddress, FromName: m.FromName,
		To: m.To, Cc: m.Cc, Bcc: m.Bcc, Subject: m.Subject, Snippet: m.Snippet, Error: m.Error,
		RelatedEntityType: string(m.RelatedEntityType), RelatedEntityID: m.RelatedEntityID,
		SentAt: m.SentAt, CreatedAt: m.CreatedAt, Attachments: attachments,
	}
	if withBody {
		response.BodyHTML = m.BodyHTML
	}
	return response
}

func MessageListFromDomain(messages []domain.Message) []MessageResponse {
	out := make([]MessageResponse, 0, len(messages))
	for _, m := range messages {
		out = append(out, MessageFromDomain(m, false))
	}
	return out
}
