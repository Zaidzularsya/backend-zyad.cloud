package model

import "time"

type UserProfile struct {
	ID         string
	UserID     string
	AvatarURL  string
	Bio        string
	JobTitle   string
	Department string
	Company    string
	Address    string
	Timezone   string
	Language   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
