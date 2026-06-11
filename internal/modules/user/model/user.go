package model

import "time"

type User struct {
	ID              string
	Name            string
	Email           string
	Username        string
	PasswordHash    string
	Phone           string
	Status          UserStatus
	EmailVerifiedAt *time.Time
	PhoneVerifiedAt *time.Time
	LastLoginAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
	Profile         *UserProfile
	Roles           []UserRole
	Permissions     []UserPermission
}

func (u User) IsDeleted() bool {
	return u.DeletedAt != nil || u.Status == UserStatusDeleted
}

func (u User) CanLogin() bool {
	return !u.IsDeleted() && u.Status.CanLogin()
}

func (u User) IsEmailVerified() bool {
	return u.EmailVerifiedAt != nil
}

func (u User) IsPhoneVerified() bool {
	return u.PhoneVerifiedAt != nil
}
