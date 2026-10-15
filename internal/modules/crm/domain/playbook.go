package domain

import (
	"slices"
	"time"
)

type DisqualifyReason string

const (
	DisqualifyUnresponsive  DisqualifyReason = "unresponsive"
	DisqualifyNotInterested DisqualifyReason = "not_interested"
	DisqualifyNotFit        DisqualifyReason = "not_fit"
	DisqualifyBudget        DisqualifyReason = "budget"
	DisqualifyCompetitor    DisqualifyReason = "competitor"
	DisqualifyBadData       DisqualifyReason = "bad_data"
	DisqualifyDuplicate     DisqualifyReason = "duplicate"
	DisqualifyBadTiming     DisqualifyReason = "bad_timing"
)

func (r DisqualifyReason) IsValid() bool {
	return slices.Contains([]DisqualifyReason{
		DisqualifyUnresponsive, DisqualifyNotInterested, DisqualifyNotFit, DisqualifyBudget,
		DisqualifyCompetitor, DisqualifyBadData, DisqualifyDuplicate, DisqualifyBadTiming,
	}, r)
}

// LeadRequirements is the "form kebutuhan" captured when a lead qualifies.
// BudgetEstimate is a decimal string (numeric(18,2)), like AnnualRevenue.
type LeadRequirements struct {
	Summary        string
	BudgetEstimate *string
	TargetDate     *time.Time
	DecisionMaker  string
}

type PlaybookInput string

const (
	PlaybookInputNone         PlaybookInput = "none"
	PlaybookInputRequirements PlaybookInput = "requirements"
	PlaybookInputDisqualify   PlaybookInput = "disqualify"
	PlaybookInputReschedule   PlaybookInput = "reschedule"
)

type OutcomeVisibility string

const (
	OutcomeVisibleNormal      OutcomeVisibility = "normal"
	OutcomeVisibleFinalReview OutcomeVisibility = "final_review"
	OutcomeVisibleBoth        OutcomeVisibility = "both"
)

type PlaybookRunStatus string

const (
	PlaybookRunActive    PlaybookRunStatus = "active"
	PlaybookRunCompleted PlaybookRunStatus = "completed"
	PlaybookRunCancelled PlaybookRunStatus = "cancelled"
)

type PlaybookRunResult string

const (
	PlaybookResultQualified    PlaybookRunResult = "qualified"
	PlaybookResultConverted    PlaybookRunResult = "converted"
	PlaybookResultDisqualified PlaybookRunResult = "disqualified"
	PlaybookResultCancelled    PlaybookRunResult = "cancelled"
)

// DueRule is stored as jsonb. Kind "business_minutes" uses Value;
// "business_days_at" uses Value (or ByAttempt[attempt-2] when set) + At.
type DueRule struct {
	Kind      string `json:"kind"`
	Value     int    `json:"value,omitempty"`
	ByAttempt []int  `json:"by_attempt,omitempty"`
	At        string `json:"at,omitempty"`
}

type PlaybookOutcome struct {
	ID               string
	StepID           string
	Key              string
	Label            string
	RequiredInput    PlaybookInput
	SetLeadStatus    *LeadStatus
	DisqualifyReason *DisqualifyReason
	NextStepKey      *string
	NextDueRule      *DueRule
	RepeatStep       bool
	EndsRun          bool
	RunResult        *PlaybookRunResult
	Visibility       OutcomeVisibility
	Position         int
}

type PlaybookStep struct {
	ID             string
	PlaybookID     string
	Key            string
	Name           string
	ActivityType   ActivityType
	DueRule        DueRule
	ChannelActions []string
	EntryStatuses  []LeadStatus
	MaxAttempts    *int
	Position       int
	Outcomes       []PlaybookOutcome // ordered by Position
}

type Playbook struct {
	ID             string
	OrganizationID *string
	EntityType     string
	Trigger        string
	Key            string
	Name           string
	Version        int
	Steps          []PlaybookStep // ordered by Position
}

type PlaybookRun struct {
	ID              string
	OrganizationID  string
	PlaybookID      string
	PlaybookVersion int
	EntityType      string
	EntityID        string
	CurrentStepID   *string
	Status          PlaybookRunStatus
	Result          *PlaybookRunResult
	StartedBy       string
	StartedAt       time.Time
	EndedAt         *time.Time
}

// ActivityPlaybookInfo is the read model attached to a playbook activity.
type ActivityPlaybookInfo struct {
	RunID          string
	StepKey        string
	StepName       string
	AttemptNo      int
	MaxAttempts    *int
	FinalReview    bool
	ChannelActions []string
	Outcomes       []PlaybookOutcome // already filtered by visibility
}

// LeadPlaybookSummary is the lead's latest run (active preferred).
type LeadPlaybookSummary struct {
	RunID       string
	Status      PlaybookRunStatus
	Result      *PlaybookRunResult
	StepKey     string
	StepName    string
	DueAt       *time.Time
	AttemptNo   int
	FinalReview bool
}
