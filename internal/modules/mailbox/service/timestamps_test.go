package service

import (
	"context"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	coretenant "zyad.cloud/internal/core/tenant"

	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
)

type cutoffRecorder struct {
	repository.MessageRepository
	cutoff time.Time
}

func (c *cutoffRecorder) FailStaleQueued(_ context.Context, _ coretenant.Scope, _ string, cutoff time.Time) error {
	c.cutoff = cutoff
	return nil
}

func (c *cutoffRecorder) List(context.Context, coretenant.Scope, repository.MessageListFilter) ([]domain.Message, int64, error) {
	return nil, 0, nil
}

// created_at is stored as UTC wall-clock; pgx keeps the wall clock of a
// non-UTC time.Time as-is, so a cutoff in the server's local zone would sit
// hours ahead of created_at and fail every queued message.
func TestListFailsStaleQueuedWithUTCCutoff(t *testing.T) {
	rec := &cutoffRecorder{}
	svc := &MessageService{messages: rec, now: func() time.Time {
		return time.Date(2026, 1, 10, 23, 30, 0, 0, time.FixedZone("WIB", 7*3600))
	}}
	if _, _, err := svc.List(context.Background(), testScope, "user-1", MessageListInput{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if rec.cutoff.Location() != time.UTC {
		t.Fatalf("cutoff location = %s, want UTC (cutoff %s)", rec.cutoff.Location(), rec.cutoff)
	}
	if want := time.Date(2026, 1, 10, 16, 30, 0, 0, time.UTC).Add(-staleQueuedAfter); !rec.cutoff.Equal(want) {
		t.Fatalf("cutoff = %s, want %s", rec.cutoff, want)
	}
}

func TestParseFetchedMessageDateIsUTC(t *testing.T) {
	sentInSenderZone := time.Date(2026, 1, 10, 23, 30, 0, 0, time.FixedZone("", 7*3600))
	got := parseFetchedMessage(&imapclient.FetchMessageBuffer{
		UID:      imap.UID(1),
		Envelope: &imap.Envelope{Date: sentInSenderZone},
	}, 1)
	if got.Date.Location() != time.UTC || !got.Date.Equal(sentInSenderZone) {
		t.Fatalf("Date = %s (%s), want the same instant in UTC", got.Date, got.Date.Location())
	}
}
