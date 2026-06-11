package model

import "time"

type AuditLog struct {
	ID           string
	Module       string
	Event        string
	ActorUserID  string
	TargetUserID string
	TargetType   string
	TargetID     string
	Metadata     map[string]any
	IPAddress    string
	UserAgent    string
	CreatedAt    time.Time
}
