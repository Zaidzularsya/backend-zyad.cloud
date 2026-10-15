package playbook

import (
	"errors"
	"slices"
	"strings"
	"time"

	"zyad.cloud/internal/modules/crm/domain"
)

var (
	ErrOutcomeRequired      = errors.New("outcome_key is required for a playbook step")
	ErrOutcomeInvalid       = errors.New("outcome_key is not valid for this step")
	ErrOutcomeInputRequired = errors.New("the chosen outcome requires additional input")
)

type DisqualifyInput struct {
	Reason domain.DisqualifyReason
	Note   string
}

type OutcomeInput struct {
	RescheduleAt *time.Time
	Requirements *domain.LeadRequirements
	Disqualify   *DisqualifyInput
}

type ResolveRequest struct {
	Playbook    domain.Playbook
	Step        domain.PlaybookStep
	AttemptNo   int
	FinalReview bool
	OutcomeKey  string
	Input       OutcomeInput
	Now         time.Time
}

type NextStep struct {
	Step        domain.PlaybookStep
	AttemptNo   int
	FinalReview bool
	DueAt       time.Time
}

// Transition is everything the repository must apply, atomically.
type Transition struct {
	Outcome          domain.PlaybookOutcome
	SetLeadStatus    *domain.LeadStatus
	DisqualifyReason *domain.DisqualifyReason
	DisqualifyNote   string
	Requirements     *domain.LeadRequirements
	Next             *NextStep // nil when the run ends
	EndRun           bool
	RunResult        *domain.PlaybookRunResult
}

const finalReviewDays = 1

// ComputeDue applies a due rule. attemptNo is the attempt the new activity
// will carry (used by ByAttempt: attempt 2 → ByAttempt[0]).
func ComputeDue(rule domain.DueRule, attemptNo int, now time.Time) time.Time {
	switch rule.Kind {
	case "business_minutes":
		return AddBusinessMinutes(now, rule.Value)
	case "business_days_at":
		days := rule.Value
		if len(rule.ByAttempt) > 0 {
			idx := min(max(attemptNo-2, 0), len(rule.ByAttempt)-1)
			days = rule.ByAttempt[idx]
		}
		return BusinessDaysAt(now, days, rule.At)
	default:
		return AddBusinessMinutes(now, 60)
	}
}

func AvailableOutcomes(step domain.PlaybookStep, finalReview bool) []domain.PlaybookOutcome {
	out := make([]domain.PlaybookOutcome, 0, len(step.Outcomes))
	for _, o := range step.Outcomes {
		switch {
		case o.Visibility == domain.OutcomeVisibleBoth,
			finalReview && o.Visibility == domain.OutcomeVisibleFinalReview,
			!finalReview && o.Visibility == domain.OutcomeVisibleNormal:
			out = append(out, o)
		}
	}
	return out
}

func StepAllowsStatus(step domain.PlaybookStep, status domain.LeadStatus) bool {
	return slices.Contains(step.EntryStatuses, status)
}

// StepForStatus returns the step a run should (re)start at for a lead in
// the given status (R8), in playbook order.
func StepForStatus(pb domain.Playbook, status domain.LeadStatus) (domain.PlaybookStep, bool) {
	for _, s := range pb.Steps {
		if StepAllowsStatus(s, status) {
			return s, true
		}
	}
	return domain.PlaybookStep{}, false
}

func findStep(pb domain.Playbook, key string) (domain.PlaybookStep, bool) {
	for _, s := range pb.Steps {
		if s.Key == key {
			return s, true
		}
	}
	return domain.PlaybookStep{}, false
}

func Resolve(req ResolveRequest) (Transition, error) {
	if strings.TrimSpace(req.OutcomeKey) == "" {
		return Transition{}, ErrOutcomeRequired
	}
	var outcome *domain.PlaybookOutcome
	for _, o := range AvailableOutcomes(req.Step, req.FinalReview) {
		if o.Key == req.OutcomeKey {
			o := o
			outcome = &o
			break
		}
	}
	if outcome == nil {
		return Transition{}, ErrOutcomeInvalid
	}

	tr := Transition{Outcome: *outcome, SetLeadStatus: outcome.SetLeadStatus, DisqualifyReason: outcome.DisqualifyReason}

	switch outcome.RequiredInput {
	case domain.PlaybookInputReschedule:
		if req.Input.RescheduleAt == nil || !req.Input.RescheduleAt.After(req.Now) {
			return Transition{}, ErrOutcomeInputRequired
		}
	case domain.PlaybookInputRequirements:
		r := req.Input.Requirements
		if r == nil || strings.TrimSpace(r.Summary) == "" {
			return Transition{}, ErrOutcomeInputRequired
		}
		trimmed := *r
		trimmed.Summary = strings.TrimSpace(r.Summary)
		trimmed.DecisionMaker = strings.TrimSpace(r.DecisionMaker)
		tr.Requirements = &trimmed
	case domain.PlaybookInputDisqualify:
		d := req.Input.Disqualify
		if d == nil || !d.Reason.IsValid() {
			return Transition{}, ErrOutcomeInputRequired
		}
		reason := d.Reason
		tr.DisqualifyReason = &reason
	}
	if req.Input.Disqualify != nil && tr.DisqualifyReason != nil {
		tr.DisqualifyNote = strings.TrimSpace(req.Input.Disqualify.Note)
	}

	switch {
	case outcome.EndsRun:
		tr.EndRun = true
		tr.RunResult = outcome.RunResult
	case outcome.RequiredInput == domain.PlaybookInputReschedule:
		tr.Next = &NextStep{Step: req.Step, AttemptNo: req.AttemptNo, FinalReview: req.FinalReview, DueAt: req.Input.RescheduleAt.UTC()}
	case outcome.RepeatStep:
		if req.Step.MaxAttempts != nil && req.AttemptNo >= *req.Step.MaxAttempts {
			tr.Next = &NextStep{Step: req.Step, AttemptNo: req.AttemptNo, FinalReview: true,
				DueAt: BusinessDaysAt(req.Now, finalReviewDays, "09:00")}
		} else {
			attempt := req.AttemptNo + 1
			rule := req.Step.DueRule
			if outcome.NextDueRule != nil {
				rule = *outcome.NextDueRule
			}
			tr.Next = &NextStep{Step: req.Step, AttemptNo: attempt, DueAt: ComputeDue(rule, attempt, req.Now)}
		}
	case outcome.NextStepKey != nil:
		next, ok := findStep(req.Playbook, *outcome.NextStepKey)
		if !ok {
			return Transition{}, errors.New("playbook definition error: unknown next_step_key " + *outcome.NextStepKey)
		}
		rule := next.DueRule
		if outcome.NextDueRule != nil {
			rule = *outcome.NextDueRule
		}
		tr.Next = &NextStep{Step: next, AttemptNo: 1, DueAt: ComputeDue(rule, 1, req.Now)}
	default:
		return Transition{}, errors.New("playbook definition error: outcome " + outcome.Key + " neither ends the run nor has a next step")
	}
	return tr, nil
}
