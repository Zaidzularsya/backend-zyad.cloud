package model

import "testing"

func TestUserStatusIsValid(t *testing.T) {
	validStatuses := []UserStatus{
		UserStatusActive,
		UserStatusInactive,
		UserStatusPending,
		UserStatusSuspended,
		UserStatusBanned,
		UserStatusDeleted,
		UserStatusInvited,
	}

	for _, status := range validStatuses {
		if !status.IsValid() {
			t.Fatalf("expected status %q to be valid", status)
		}
	}

	if UserStatus("unknown").IsValid() {
		t.Fatal("expected unknown status to be invalid")
	}
}

func TestUserCanLogin(t *testing.T) {
	user := User{Status: UserStatusActive}
	if !user.CanLogin() {
		t.Fatal("expected active user to be able to login")
	}

	user.Status = UserStatusSuspended
	if user.CanLogin() {
		t.Fatal("expected suspended user to be unable to login")
	}
}
