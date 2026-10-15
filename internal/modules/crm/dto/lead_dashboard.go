package dto

import (
	"time"

	"zyad.cloud/internal/modules/crm/domain"
)

// LeadDashboardQuery binds GET /app/crm/leads/dashboard. Dates are
// YYYY-MM-DD (Asia/Jakarta calendar days, both inclusive).
type LeadDashboardQuery struct {
	From        string `form:"from"`
	To          string `form:"to"`
	Granularity string `form:"granularity"`
}

const dateLayout = "2006-01-02"

var leadStatusOrder = []domain.LeadStatus{
	domain.LeadStatusNew,
	domain.LeadStatusAttempting,
	domain.LeadStatusContacted,
	domain.LeadStatusQualified,
	domain.LeadStatusUnqualified,
	domain.LeadStatusConverted,
}

type LeadDashboardResponse struct {
	Range             LeadDashboardRangeResponse    `json:"range"`
	StatusCounts      map[string]int64              `json:"status_counts"`
	StatusEntered     map[string]LeadPeriodCount    `json:"status_entered"`
	Created           LeadPeriodCount               `json:"created"`
	Converted         LeadPeriodCount               `json:"converted"`
	Series            []LeadDashboardBucketResponse `json:"series"`
	BySource          []LeadSourceCountResponse     `json:"by_source"`
	FollowUpSummary   LeadFollowUpSummaryResponse   `json:"follow_up_summary"`
	UpcomingFollowUps []LeadFollowUpResponse        `json:"upcoming_follow_ups"`
	RecentActivity    []LeadActivityItemResponse    `json:"recent_activity"`
}

type LeadDashboardRangeResponse struct {
	From         string `json:"from"`
	To           string `json:"to"`
	PreviousFrom string `json:"previous_from"`
	PreviousTo   string `json:"previous_to"`
	Granularity  string `json:"granularity"`
}

// LeadPeriodCount pairs a count in the selected range with the same count
// in the previous range of equal length.
type LeadPeriodCount struct {
	Current  int64 `json:"current"`
	Previous int64 `json:"previous"`
}

type LeadDashboardBucketResponse struct {
	// Bucket is the first day of the day/month bucket (YYYY-MM-DD).
	Bucket    string `json:"bucket"`
	Created   int64  `json:"created"`
	Converted int64  `json:"converted"`
}

type LeadSourceCountResponse struct {
	// Source "" = lead without a source.
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

type LeadFollowUpSummaryResponse struct {
	Pending      int64 `json:"pending"`
	Overdue      int64 `json:"overdue"`
	DueToday     int64 `json:"due_today"`
	DueNext7Days int64 `json:"due_next_7_days"`
}

type LeadFollowUpResponse struct {
	ActivityResponse
	LeadName     string `json:"lead_name"`
	CompanyName  string `json:"company_name,omitempty"`
	AssigneeName string `json:"assignee_name,omitempty"`
	StepName     string `json:"step_name,omitempty"`
}

type LeadActivityItemResponse struct {
	// Kind: created | status_changed | assigned | converted | deleted |
	// restored | activity_created | activity_completed.
	Kind         string    `json:"kind"`
	OccurredAt   time.Time `json:"occurred_at"`
	LeadID       string    `json:"lead_id"`
	LeadName     string    `json:"lead_name"`
	ActorUserID  string    `json:"actor_user_id,omitempty"`
	ActorName    string    `json:"actor_name,omitempty"`
	FromValue    string    `json:"from_value,omitempty"`
	ToValue      string    `json:"to_value,omitempty"`
	FromName     string    `json:"from_name,omitempty"`
	ToName       string    `json:"to_name,omitempty"`
	ActivityID   string    `json:"activity_id,omitempty"`
	ActivityType string    `json:"activity_type,omitempty"`
	Subject      string    `json:"subject,omitempty"`
}

func LeadDashboardFromDomain(d domain.LeadDashboard, rng domain.LeadDashboardRange) LeadDashboardResponse {
	resp := LeadDashboardResponse{
		Range: LeadDashboardRangeResponse{
			From:         rng.From.Format(dateLayout),
			To:           rng.To.Format(dateLayout),
			PreviousFrom: rng.PreviousFrom.Format(dateLayout),
			PreviousTo:   rng.PreviousTo.Format(dateLayout),
			Granularity:  string(rng.Granularity),
		},
		StatusCounts:      make(map[string]int64, len(leadStatusOrder)),
		StatusEntered:     make(map[string]LeadPeriodCount, len(leadStatusOrder)),
		Created:           LeadPeriodCount{Current: d.Created, Previous: d.PreviousCreated},
		Converted:         LeadPeriodCount{Current: d.Converted, Previous: d.PreviousConverted},
		Series:            make([]LeadDashboardBucketResponse, 0, len(d.Series)),
		BySource:          make([]LeadSourceCountResponse, 0, len(d.BySource)),
		UpcomingFollowUps: make([]LeadFollowUpResponse, 0, len(d.UpcomingFollowUps)),
		RecentActivity:    make([]LeadActivityItemResponse, 0, len(d.RecentActivity)),
		FollowUpSummary: LeadFollowUpSummaryResponse{
			Pending:      d.FollowUps.Pending,
			Overdue:      d.FollowUps.Overdue,
			DueToday:     d.FollowUps.DueToday,
			DueNext7Days: d.FollowUps.DueNext7Days,
		},
	}

	// Every status is always present so the client never has to default.
	for _, s := range leadStatusOrder {
		resp.StatusCounts[string(s)] = d.StatusCounts[s]
		resp.StatusEntered[string(s)] = LeadPeriodCount{Current: d.StatusEntered[s], Previous: d.PreviousStatusEntered[s]}
	}
	for _, b := range d.Series {
		resp.Series = append(resp.Series, LeadDashboardBucketResponse{Bucket: b.Bucket.Format(dateLayout), Created: b.Created, Converted: b.Converted})
	}
	for _, s := range d.BySource {
		resp.BySource = append(resp.BySource, LeadSourceCountResponse{Source: s.Source, Count: s.Count})
	}
	for _, f := range d.UpcomingFollowUps {
		resp.UpcomingFollowUps = append(resp.UpcomingFollowUps, LeadFollowUpResponse{
			ActivityResponse: ActivityFromDomain(f.Activity),
			LeadName:         f.LeadName,
			CompanyName:      f.CompanyName,
			AssigneeName:     f.AssigneeName,
			StepName:         f.StepName,
		})
	}
	for _, it := range d.RecentActivity {
		resp.RecentActivity = append(resp.RecentActivity, LeadActivityItemResponse{
			Kind:         it.Kind,
			OccurredAt:   it.OccurredAt,
			LeadID:       it.LeadID,
			LeadName:     it.LeadName,
			ActorUserID:  it.ActorUserID,
			ActorName:    it.ActorName,
			FromValue:    it.FromValue,
			ToValue:      it.ToValue,
			FromName:     it.FromName,
			ToName:       it.ToName,
			ActivityID:   it.ActivityID,
			ActivityType: string(it.ActivityType),
			Subject:      it.Subject,
		})
	}
	return resp
}
