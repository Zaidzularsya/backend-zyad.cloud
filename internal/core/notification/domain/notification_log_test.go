package domain

import (
	"testing"
	"time"
)

func TestLogStatusIsValid(t *testing.T) {
	valid := []LogStatus{
		LogStatusPending,
		LogStatusProcessing,
		LogStatusSent,
		LogStatusFailed,
		LogStatusCancelled,
		LogStatusDead,
	}

	for _, status := range valid {
		if !status.IsValid() {
			t.Fatalf("expected status %q to be valid", status)
		}
	}

	if LogStatus("unknown").IsValid() {
		t.Fatal("expected unknown status to be invalid")
	}
}

func TestNotificationLogCanRetry(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	tests := []struct {
		name string
		log  NotificationLog
		want bool
	}{
		{
			name: "pending without schedule can retry",
			log:  NotificationLog{Status: LogStatusPending, Attempts: 0, MaxAttempts: 3},
			want: true,
		},
		{
			name: "failed scheduled in past can retry",
			log:  NotificationLog{Status: LogStatusFailed, Attempts: 1, MaxAttempts: 3, NextRetryAt: &past},
			want: true,
		},
		{
			name: "failed scheduled in future cannot retry",
			log:  NotificationLog{Status: LogStatusFailed, Attempts: 1, MaxAttempts: 3, NextRetryAt: &future},
			want: false,
		},
		{
			name: "sent cannot retry",
			log:  NotificationLog{Status: LogStatusSent, Attempts: 0, MaxAttempts: 3},
			want: false,
		},
		{
			name: "max attempts cannot retry",
			log:  NotificationLog{Status: LogStatusFailed, Attempts: 3, MaxAttempts: 3},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.log.CanRetry(now); got != tt.want {
				t.Fatalf("CanRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}
