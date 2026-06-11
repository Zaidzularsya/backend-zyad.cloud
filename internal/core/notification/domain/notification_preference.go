package domain

import "time"

type NotificationPreference struct {
	ID             string
	UserID         string
	OrganizationID string
	EventType      string
	Channel        Channel
	IsEnabled      bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (p NotificationPreference) Matches(userID, organizationID, eventType string, channel Channel) bool {
	return p.UserID == userID &&
		p.OrganizationID == organizationID &&
		p.EventType == eventType &&
		p.Channel == channel
}
