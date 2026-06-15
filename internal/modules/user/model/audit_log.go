package model

import "time"

type AuditLog struct {
	ID                     string
	Module                 string
	Event                  string
	OrganizationID         string
	MembershipID           string
	SessionID              string
	ActorUserID            string
	OperatorUserID         string
	EffectiveUserID        string
	ImpersonationSessionID string
	ResolutionSource       string
	RequestID              string
	TargetUserID           string
	TargetType             string
	TargetID               string
	Metadata               map[string]any
	IPAddress              string
	UserAgent              string
	CreatedAt              time.Time
}
