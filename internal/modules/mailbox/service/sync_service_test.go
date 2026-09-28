package service

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	corecrypto "zyad.cloud/internal/core/crypto"
	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
)

const syncOrgID = "11111111-1111-1111-1111-111111111111"

func syncScopeContext(t *testing.T, orgID string) coretenant.Context {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: orgID, OrganizationSlug: "org",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource: coretenant.ResolutionSourceWorker, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("tenant context: %v", err)
	}
	return tenantContext
}

type fakeSyncResolver struct {
	scopes map[string]coretenant.Context
}

func (r *fakeSyncResolver) ResolveWorkerOrganization(_ context.Context, orgID, _ string) (coretenant.Context, error) {
	tenantContext, ok := r.scopes[orgID]
	if !ok {
		return coretenant.Context{}, errors.New("organization not found")
	}
	return tenantContext, nil
}

type fakeSyncMailboxes struct {
	repository.MailboxRepository
	byID          map[string]domain.Mailbox
	statusCalls   []domain.MailboxStatus
	cursorUpdates []domain.SyncCursor
}

func (f *fakeSyncMailboxes) GetByID(_ context.Context, _ coretenant.Scope, id string) (domain.Mailbox, error) {
	mailbox, ok := f.byID[id]
	if !ok {
		return domain.Mailbox{}, pgx.ErrNoRows
	}
	return mailbox, nil
}

func (f *fakeSyncMailboxes) UpdateSyncCursor(_ context.Context, _ coretenant.Scope, id string, cursor domain.SyncCursor, _ time.Time) error {
	f.cursorUpdates = append(f.cursorUpdates, cursor)
	mailbox := f.byID[id]
	mailbox.SyncCursor = cursor
	mailbox.Status = domain.MailboxStatusActive
	f.byID[id] = mailbox
	return nil
}

func (f *fakeSyncMailboxes) SetStatus(_ context.Context, _ coretenant.Scope, id string, status domain.MailboxStatus, lastError string) error {
	f.statusCalls = append(f.statusCalls, status)
	mailbox := f.byID[id]
	mailbox.Status, mailbox.LastError = status, lastError
	f.byID[id] = mailbox
	return nil
}

type fakeSyncMessages struct {
	repository.MessageRepository
	byKey       map[string]domain.Message
	seq         int
	failOnUID   uint32
	attachments map[string][]string
}

func newFakeSyncMessages() *fakeSyncMessages {
	return &fakeSyncMessages{byKey: map[string]domain.Message{}, attachments: map[string][]string{}}
}

func (f *fakeSyncMessages) CreateInbound(_ context.Context, _ coretenant.Scope, p repository.CreateInboundParams) (domain.Message, bool, error) {
	if f.failOnUID != 0 && p.Subject == uidSubject(f.failOnUID) {
		return domain.Message{}, false, errors.New("db unavailable")
	}
	key := p.MailboxID + "|" + p.MessageID
	if existing, ok := f.byKey[key]; ok {
		return existing, false, nil
	}
	f.seq++
	message := domain.Message{ID: "msg-" + string(rune('0'+f.seq)), MailboxID: p.MailboxID, MessageID: p.MessageID, Subject: p.Subject}
	f.byKey[key] = message
	return message, true, nil
}

func (f *fakeSyncMessages) AddAttachment(_ context.Context, _ coretenant.Scope, messageID, assetObjectID string) error {
	f.attachments[messageID] = append(f.attachments[messageID], assetObjectID)
	return nil
}

type fakeSyncStorage struct {
	seq         int
	uploadErr   error
	deletedIDs  []string
	uploadCount int
}

func (f *fakeSyncStorage) UploadObject(_ context.Context, _ coretenant.Scope, params assetservice.UploadObjectParams, _ io.Reader) (assetdomain.AssetObject, error) {
	if f.uploadErr != nil {
		return assetdomain.AssetObject{}, f.uploadErr
	}
	if !params.AllowedMimeTypes[params.MimeType] {
		return assetdomain.AssetObject{}, assetservice.ErrInvalidMimeType
	}
	f.seq++
	f.uploadCount++
	return assetdomain.AssetObject{ID: "obj-" + string(rune('0'+f.seq))}, nil
}

func (f *fakeSyncStorage) DeleteObject(_ context.Context, _ coretenant.Scope, id string) error {
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

func (f *fakeSyncStorage) DownloadURL(context.Context, coretenant.Scope, string) (assetservice.DownloadResult, error) {
	return assetservice.DownloadResult{}, nil
}

type fakeIMAPConn struct {
	uidValidity uint32
	messages    []FetchedMessage
	fetchErr    error
	closed      bool
	gotAfterUID uint32
	gotCutoff   time.Time
}

func (c *fakeIMAPConn) UIDValidity() uint32 { return c.uidValidity }

func (c *fakeIMAPConn) FetchSince(_ context.Context, afterUID uint32, cutoff time.Time, limit int) ([]FetchedMessage, error) {
	c.gotAfterUID, c.gotCutoff = afterUID, cutoff
	if c.fetchErr != nil {
		return nil, c.fetchErr
	}
	out := []FetchedMessage{}
	for _, message := range c.messages {
		if message.UID > afterUID {
			out = append(out, message)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (c *fakeIMAPConn) Close() error { c.closed = true; return nil }

func uidSubject(uid uint32) string { return "subject-" + string(rune('0'+uid)) }

func newSyncFixture(t *testing.T, conn *fakeIMAPConn, dialErr error) (*SyncService, *fakeSyncMailboxes, *fakeSyncMessages, *fakeSyncStorage) {
	t.Helper()
	mailbox := domain.Mailbox{
		ID: "mb-1", UserID: "user-1", EmailAddress: "sales@zyad.test",
		IMAPHost: "imap.zyad.test", IMAPPort: 993, IMAPSecurity: domain.SecuritySSL,
		Username: "sales@zyad.test", SecretEncrypted: mustEncrypt(t, "app-password"),
		Status: domain.MailboxStatusActive, SyncCursor: domain.SyncCursor{},
	}
	mailboxes := &fakeSyncMailboxes{byID: map[string]domain.Mailbox{"mb-1": mailbox}}
	messages := newFakeSyncMessages()
	storage := &fakeSyncStorage{}
	directory := fakeSyncDirectory{{MailboxID: "mb-1", OrganizationID: syncOrgID}}
	resolver := &fakeSyncResolver{scopes: map[string]coretenant.Context{syncOrgID: syncScopeContext(t, syncOrgID)}}

	svc := NewSyncService(directory, mailboxes, messages, storage, resolver, SyncServiceOptions{
		SecretKey: testSecret, InitialSyncDays: 30, BatchLimit: 200,
		Dial: func(_ context.Context, cfg IMAPDialConfig, _ bool) (IMAPConn, error) {
			if cfg.Password != "app-password" {
				t.Errorf("dial password = %q", cfg.Password)
			}
			if dialErr != nil {
				return nil, dialErr
			}
			return conn, nil
		},
		Now: func() time.Time { return time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC) },
	})
	return svc, mailboxes, messages, storage
}

type fakeSyncDirectory []repository.DirectoryEntry

func (d fakeSyncDirectory) ListAll(context.Context) ([]repository.DirectoryEntry, error) {
	return d, nil
}

func mustEncrypt(t *testing.T, plaintext string) string {
	t.Helper()
	encrypted, err := corecrypto.EncryptSecret(testSecret, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	return encrypted
}

func TestSyncServiceStoresNewMessagesAndAdvancesCursor(t *testing.T) {
	conn := &fakeIMAPConn{uidValidity: 42, messages: []FetchedMessage{
		{UID: 1, MessageID: "<a@x>", Subject: uidSubject(1), Attachments: []FetchedAttachment{{Filename: "a.pdf", ContentType: "application/pdf", Content: []byte("x")}}},
		{UID: 2, MessageID: "<b@x>", Subject: uidSubject(2)},
	}}
	svc, mailboxes, messages, storage := newSyncFixture(t, conn, nil)

	result, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Checked != 1 || result.Synced != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(messages.byKey) != 2 {
		t.Fatalf("stored %d messages, want 2", len(messages.byKey))
	}
	if storage.uploadCount != 1 {
		t.Fatalf("uploaded %d attachments, want 1", storage.uploadCount)
	}
	cursor := mailboxes.byID["mb-1"].SyncCursor[syncFolder]
	if cursor.LastUID != 2 || cursor.UIDValidity != 42 {
		t.Fatalf("cursor = %+v, want last_uid=2 uidvalidity=42", cursor)
	}
	if !conn.closed {
		t.Error("imap connection not closed")
	}
}

func TestSyncServiceOnlyFetchesAfterCursorOnSubsequentRuns(t *testing.T) {
	conn := &fakeIMAPConn{uidValidity: 42, messages: []FetchedMessage{{UID: 3, MessageID: "<c@x>", Subject: uidSubject(3)}}}
	svc, mailboxes, _, _ := newSyncFixture(t, conn, nil)
	mailboxes.byID["mb-1"] = withCursor(mailboxes.byID["mb-1"], domain.FolderCursor{UIDValidity: 42, LastUID: 2})

	if _, err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if conn.gotAfterUID != 2 {
		t.Fatalf("FetchSince afterUID = %d, want 2", conn.gotAfterUID)
	}
	if got := mailboxes.byID["mb-1"].SyncCursor[syncFolder].LastUID; got != 3 {
		t.Fatalf("cursor advanced to %d, want 3", got)
	}
}

func TestSyncServiceResyncsWhenUIDValidityChanges(t *testing.T) {
	conn := &fakeIMAPConn{uidValidity: 99, messages: []FetchedMessage{{UID: 1, MessageID: "<a@x>", Subject: uidSubject(1)}}}
	svc, mailboxes, _, _ := newSyncFixture(t, conn, nil)
	mailboxes.byID["mb-1"] = withCursor(mailboxes.byID["mb-1"], domain.FolderCursor{UIDValidity: 42, LastUID: 999})

	if _, err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	// Old UIDs are meaningless after a UIDVALIDITY change: fetch again from
	// scratch (afterUID 0), bounded by the initial-sync cutoff.
	if conn.gotAfterUID != 0 {
		t.Fatalf("FetchSince afterUID = %d, want 0 (resync)", conn.gotAfterUID)
	}
	if conn.gotCutoff.IsZero() {
		t.Error("resync did not use the initial-sync cutoff")
	}
}

func TestSyncServiceIsIdempotent(t *testing.T) {
	conn := &fakeIMAPConn{uidValidity: 42, messages: []FetchedMessage{
		{UID: 1, MessageID: "<a@x>", Subject: uidSubject(1), Attachments: []FetchedAttachment{{Filename: "a.pdf", ContentType: "application/pdf", Content: []byte("x")}}},
	}}
	svc, _, messages, storage := newSyncFixture(t, conn, nil)

	if _, err := svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	conn.messages = append(conn.messages, FetchedMessage{UID: 1, MessageID: "<a@x>", Subject: uidSubject(1)})
	if _, err := svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(messages.byKey) != 1 {
		t.Fatalf("stored %d messages after resync, want 1 (deduped)", len(messages.byKey))
	}
	if storage.uploadCount != 1 {
		t.Fatalf("uploaded %d attachments, want 1 (not re-uploaded on dedup)", storage.uploadCount)
	}
}

func TestSyncServiceSkipsMailboxWithoutIMAP(t *testing.T) {
	conn := &fakeIMAPConn{}
	svc, mailboxes, _, _ := newSyncFixture(t, conn, nil)
	mailbox := mailboxes.byID["mb-1"]
	mailbox.IMAPHost = ""
	mailboxes.byID["mb-1"] = mailbox

	result, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Checked != 0 || result.Failed != 0 {
		t.Fatalf("result = %+v, want a mailbox without imap to be silently skipped", result)
	}
}

func TestSyncServiceMarksMailboxErrorOnDialFailure(t *testing.T) {
	svc, mailboxes, _, _ := newSyncFixture(t, &fakeIMAPConn{}, errors.New("535 auth failed"))

	result, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Checked != 1 || result.Failed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if mailboxes.byID["mb-1"].Status != domain.MailboxStatusError {
		t.Fatalf("status = %s, want error", mailboxes.byID["mb-1"].Status)
	}
}

func TestSyncServiceKeepsCursorProgressOnPartialFailure(t *testing.T) {
	conn := &fakeIMAPConn{uidValidity: 42, messages: []FetchedMessage{
		{UID: 1, MessageID: "<a@x>", Subject: uidSubject(1)},
		{UID: 2, MessageID: "<b@x>", Subject: uidSubject(2)},
		{UID: 3, MessageID: "<c@x>", Subject: uidSubject(3)},
	}}
	svc, mailboxes, messages, _ := newSyncFixture(t, conn, nil)
	messages.failOnUID = 2

	result, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("result = %+v, want the mailbox counted as failed", result)
	}
	if got := mailboxes.byID["mb-1"].SyncCursor[syncFolder].LastUID; got != 1 {
		t.Fatalf("cursor = %d, want 1 (stopped before the failed message)", got)
	}
	if len(messages.byKey) != 1 {
		t.Fatalf("stored %d messages, want 1", len(messages.byKey))
	}
}

func withCursor(mailbox domain.Mailbox, cursor domain.FolderCursor) domain.Mailbox {
	mailbox.SyncCursor = domain.SyncCursor{syncFolder: cursor}
	return mailbox
}
