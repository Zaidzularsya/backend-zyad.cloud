package domain

type Notification struct {
	EventID        string
	EventType      string
	TemplateCode   string
	OrganizationID string
	Channel        Channel
	UserID         string
	Recipient      NotificationRecipient
	Payload        map[string]any
	Locale         string
}

type NotificationRecipient struct {
	Type        string
	UserID      string
	Name        string
	Email       string
	Phone       string
	Destination string
}
