package domain

import "testing"

func TestMessageStatusCanTransitionTo(t *testing.T) {
	tests := []struct {
		from, to MessageStatus
		want     bool
	}{
		{MessageStatusPending, MessageStatusSent, true},
		{MessageStatusSent, MessageStatusDelivered, true},
		{MessageStatusSent, MessageStatusRead, true},
		{MessageStatusDelivered, MessageStatusRead, true},
		{MessageStatusRead, MessageStatusDelivered, false},
		{MessageStatusDelivered, MessageStatusSent, false},
		{MessageStatusSent, MessageStatusSent, false},
		{MessageStatusPending, MessageStatusFailed, true},
		{MessageStatusSent, MessageStatusFailed, false},
		{MessageStatusFailed, MessageStatusPending, true},
		{MessageStatusFailed, MessageStatusSent, false},
		{MessageStatusPending, MessageStatus("bogus"), false},
	}
	for _, tt := range tests {
		if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
			t.Errorf("%s -> %s = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestMessageStatusFromAck(t *testing.T) {
	tests := map[int]MessageStatus{
		-1: MessageStatusFailed,
		0:  MessageStatusPending,
		1:  MessageStatusSent,
		2:  MessageStatusDelivered,
		3:  MessageStatusRead,
		4:  MessageStatusRead,
	}
	for ack, want := range tests {
		got, ok := MessageStatusFromAck(ack)
		if !ok || got != want {
			t.Errorf("MessageStatusFromAck(%d) = %q, %v; want %q", ack, got, ok, want)
		}
	}
	if _, ok := MessageStatusFromAck(9); ok {
		t.Error("MessageStatusFromAck(9) ok = true, want false")
	}
}
