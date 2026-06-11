package model

type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusInactive  UserStatus = "inactive"
	UserStatusPending   UserStatus = "pending"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusBanned    UserStatus = "banned"
	UserStatusDeleted   UserStatus = "deleted"
	UserStatusInvited   UserStatus = "invited"
)

func (s UserStatus) IsValid() bool {
	switch s {
	case UserStatusActive,
		UserStatusInactive,
		UserStatusPending,
		UserStatusSuspended,
		UserStatusBanned,
		UserStatusDeleted,
		UserStatusInvited:
		return true
	default:
		return false
	}
}

func (s UserStatus) CanLogin() bool {
	return s == UserStatusActive
}
