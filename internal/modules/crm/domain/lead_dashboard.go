package domain

import "time"

// LeadEventType is a recorded change on a lead (crm_lead_events).
type LeadEventType string

const (
	LeadEventCreated         LeadEventType = "created"
	LeadEventStatusChanged   LeadEventType = "status_changed"
	LeadEventAssigned        LeadEventType = "assigned"
	LeadEventConverted       LeadEventType = "converted"
	LeadEventDeleted         LeadEventType = "deleted"
	LeadEventRestored        LeadEventType = "restored"
	LeadEventPlaybookStarted LeadEventType = "playbook_started"
	LeadEventPlaybookEnded   LeadEventType = "playbook_ended"
)

type LeadDashboardGranularity string

const (
	LeadDashboardDaily   LeadDashboardGranularity = "day"
	LeadDashboardMonthly LeadDashboardGranularity = "month"
)

func (g LeadDashboardGranularity) IsValid() bool {
	return g == LeadDashboardDaily || g == LeadDashboardMonthly
}

// LeadDashboardRange is an inclusive date range [From, To] (calendar days in
// the database timezone, Asia/Jakarta). Previous is the equally long range
// directly before it, used for trend deltas.
type LeadDashboardRange struct {
	From         time.Time
	To           time.Time
	PreviousFrom time.Time
	PreviousTo   time.Time
	Granularity  LeadDashboardGranularity
}

type LeadDashboard struct {
	// StatusCounts: current status of all non-deleted leads.
	StatusCounts map[LeadStatus]int64
	// StatusEntered / PreviousStatusEntered: how many leads entered each
	// status during the range / the previous range (from crm_lead_events).
	StatusEntered         map[LeadStatus]int64
	PreviousStatusEntered map[LeadStatus]int64
	Created               int64
	Converted             int64
	PreviousCreated       int64
	PreviousConverted     int64
	Series                []LeadDashboardBucket
	BySource              []LeadSourceCount
	FollowUps             LeadFollowUpSummary
	UpcomingFollowUps     []LeadFollowUp
	RecentActivity        []LeadActivityItem
}

type LeadDashboardBucket struct {
	Bucket    time.Time
	Created   int64
	Converted int64
}

// LeadSourceCount: Source "" means the lead has no source.
type LeadSourceCount struct {
	Source string
	Count  int64
}

// LeadFollowUpSummary counts pending activities on non-deleted leads.
type LeadFollowUpSummary struct {
	Pending  int64
	Overdue  int64
	DueToday int64
	// DueNext7Days excludes overdue and today.
	DueNext7Days int64
}

type LeadFollowUp struct {
	Activity     Activity
	LeadName     string
	CompanyName  string
	AssigneeName string
}

// LeadActivityItem is one entry of the merged recent-activity feed: either a
// lead event or a CRM activity on a lead.
type LeadActivityItem struct {
	// Kind: a LeadEventType value, "activity_created" or "activity_completed".
	Kind        string
	OccurredAt  time.Time
	LeadID      string
	LeadName    string
	ActorUserID string
	ActorName   string
	FromValue   string
	ToValue     string
	// FromName/ToName resolve owner user ids for "assigned" events.
	FromName     string
	ToName       string
	ActivityID   string
	ActivityType ActivityType
	Subject      string
}

// LeadEvent is one crm_lead_events row for the lead detail history.
type LeadEvent struct {
	ID          string
	LeadID      string
	EventType   LeadEventType
	FromValue   string
	ToValue     string
	ActorUserID string
	ActorName   string
	FromName    string // owner name for 'assigned'
	ToName      string
	CreatedAt   time.Time
}
