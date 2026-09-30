package playbook

import (
	"errors"
	"testing"

	"zyad.cloud/internal/modules/crm/domain"
)

func ptr[T any](v T) *T { return &v }

// defaultPlaybook mirrors the migration 000136 seed.
func defaultPlaybook() domain.Playbook {
	contacted, attempting, qualified, unqualified :=
		domain.LeadStatusContacted, domain.LeadStatusAttempting, domain.LeadStatusQualified, domain.LeadStatusUnqualified
	return domain.Playbook{ID: "pb", Key: "default_lead_sop", Version: 1, Steps: []domain.PlaybookStep{
		{
			ID: "s1", Key: "first_contact", Name: "Kontak pertama", ActivityType: domain.ActivityTypeCall,
			DueRule:       domain.DueRule{Kind: "business_minutes", Value: 60},
			EntryStatuses: []domain.LeadStatus{domain.LeadStatusNew, domain.LeadStatusAttempting},
			MaxAttempts:   ptr(3), Position: 1,
			Outcomes: []domain.PlaybookOutcome{
				{Key: "connected", RequiredInput: domain.PlaybookInputNone, SetLeadStatus: &contacted, NextStepKey: ptr("discovery"), Visibility: domain.OutcomeVisibleBoth},
				{Key: "no_response", RequiredInput: domain.PlaybookInputNone, SetLeadStatus: &attempting, RepeatStep: true,
					NextDueRule: &domain.DueRule{Kind: "business_days_at", ByAttempt: []int{1, 3}, At: "09:00"}, Visibility: domain.OutcomeVisibleNormal},
				{Key: "call_back_later", RequiredInput: domain.PlaybookInputReschedule, Visibility: domain.OutcomeVisibleNormal},
				{Key: "bad_data", RequiredInput: domain.PlaybookInputNone, SetLeadStatus: &unqualified, DisqualifyReason: ptr(domain.DisqualifyBadData),
					EndsRun: true, RunResult: ptr(domain.PlaybookResultDisqualified), Visibility: domain.OutcomeVisibleBoth},
				{Key: "give_up", RequiredInput: domain.PlaybookInputNone, SetLeadStatus: &unqualified, DisqualifyReason: ptr(domain.DisqualifyUnresponsive),
					EndsRun: true, RunResult: ptr(domain.PlaybookResultDisqualified), Visibility: domain.OutcomeVisibleFinalReview},
			},
		},
		{
			ID: "s2", Key: "discovery", Name: "Gali kebutuhan", ActivityType: domain.ActivityTypeMeeting,
			DueRule:       domain.DueRule{Kind: "business_days_at", Value: 1, At: "09:00"},
			EntryStatuses: []domain.LeadStatus{domain.LeadStatusContacted}, Position: 2,
			Outcomes: []domain.PlaybookOutcome{
				{Key: "qualified", RequiredInput: domain.PlaybookInputRequirements, SetLeadStatus: &qualified,
					EndsRun: true, RunResult: ptr(domain.PlaybookResultQualified), Visibility: domain.OutcomeVisibleNormal},
				{Key: "bad_timing", RequiredInput: domain.PlaybookInputReschedule, Visibility: domain.OutcomeVisibleNormal},
				{Key: "not_fit", RequiredInput: domain.PlaybookInputDisqualify, SetLeadStatus: &unqualified,
					EndsRun: true, RunResult: ptr(domain.PlaybookResultDisqualified), Visibility: domain.OutcomeVisibleNormal},
			},
		},
	}}
}

var now = wib("2026-09-30 10:00") // Rabu

func resolve(t *testing.T, stepIdx, attempt int, final bool, key string, in OutcomeInput) Transition {
	t.Helper()
	pb := defaultPlaybook()
	tr, err := Resolve(ResolveRequest{Playbook: pb, Step: pb.Steps[stepIdx], AttemptNo: attempt, FinalReview: final, OutcomeKey: key, Input: in, Now: now})
	if err != nil {
		t.Fatalf("Resolve(%s): %v", key, err)
	}
	return tr
}

func TestResolveConnectedMovesToDiscovery(t *testing.T) {
	tr := resolve(t, 0, 1, false, "connected", OutcomeInput{})
	if *tr.SetLeadStatus != domain.LeadStatusContacted || tr.EndRun || tr.Next == nil || tr.Next.Step.Key != "discovery" || tr.Next.AttemptNo != 1 {
		t.Fatalf("unexpected %+v", tr)
	}
	if !tr.Next.DueAt.Equal(wib("2026-10-01 09:00")) {
		t.Fatalf("due %s", tr.Next.DueAt.In(BusinessLocation))
	}
}

func TestResolveNoResponseRetriesThenFinalReview(t *testing.T) {
	tr := resolve(t, 0, 1, false, "no_response", OutcomeInput{})
	if tr.Next.AttemptNo != 2 || tr.Next.FinalReview || !tr.Next.DueAt.Equal(wib("2026-10-01 09:00")) {
		t.Fatalf("attempt 2: %+v", tr.Next)
	}
	tr = resolve(t, 0, 2, false, "no_response", OutcomeInput{})
	if tr.Next.AttemptNo != 3 || !tr.Next.DueAt.Equal(wib("2026-10-05 09:00")) {
		t.Fatalf("attempt 3: %+v", tr.Next)
	}
	tr = resolve(t, 0, 3, false, "no_response", OutcomeInput{})
	if tr.Next.AttemptNo != 3 || !tr.Next.FinalReview || tr.EndRun || !tr.Next.DueAt.Equal(wib("2026-10-01 09:00")) {
		t.Fatalf("final review: %+v", tr.Next)
	}
	if *tr.SetLeadStatus != domain.LeadStatusAttempting {
		t.Fatalf("status %s", *tr.SetLeadStatus)
	}
}

func TestResolveFinalReviewOutcomes(t *testing.T) {
	pb := defaultPlaybook()
	_, err := Resolve(ResolveRequest{Playbook: pb, Step: pb.Steps[0], AttemptNo: 3, FinalReview: true, OutcomeKey: "no_response", Now: now})
	if !errors.Is(err, ErrOutcomeInvalid) {
		t.Fatalf("no_response on final review: err=%v", err)
	}
	tr := resolve(t, 0, 3, true, "give_up", OutcomeInput{})
	if !tr.EndRun || *tr.RunResult != domain.PlaybookResultDisqualified || *tr.DisqualifyReason != domain.DisqualifyUnresponsive {
		t.Fatalf("give_up %+v", tr)
	}
	if _, err := Resolve(ResolveRequest{Playbook: pb, Step: pb.Steps[0], AttemptNo: 1, OutcomeKey: "give_up", Now: now}); !errors.Is(err, ErrOutcomeInvalid) {
		t.Fatalf("give_up outside final review: err=%v", err)
	}
}

func TestResolveRescheduleKeepsAttempt(t *testing.T) {
	at := wib("2026-10-07 14:00")
	tr := resolve(t, 0, 2, false, "call_back_later", OutcomeInput{RescheduleAt: &at})
	if tr.SetLeadStatus != nil || tr.Next.AttemptNo != 2 || tr.Next.Step.Key != "first_contact" || !tr.Next.DueAt.Equal(at) {
		t.Fatalf("%+v", tr)
	}
	past := wib("2026-09-29 10:00")
	pb := defaultPlaybook()
	for _, in := range []OutcomeInput{{}, {RescheduleAt: &past}} {
		if _, err := Resolve(ResolveRequest{Playbook: pb, Step: pb.Steps[1], AttemptNo: 1, OutcomeKey: "bad_timing", Input: in, Now: now}); !errors.Is(err, ErrOutcomeInputRequired) {
			t.Fatalf("reschedule input %+v: err=%v", in, err)
		}
	}
}

func TestResolveQualifiedRequiresSummary(t *testing.T) {
	pb := defaultPlaybook()
	_, err := Resolve(ResolveRequest{Playbook: pb, Step: pb.Steps[1], AttemptNo: 1, OutcomeKey: "qualified",
		Input: OutcomeInput{Requirements: &domain.LeadRequirements{Summary: "   "}}, Now: now})
	if !errors.Is(err, ErrOutcomeInputRequired) {
		t.Fatalf("blank summary: err=%v", err)
	}
	tr := resolve(t, 1, 1, false, "qualified", OutcomeInput{Requirements: &domain.LeadRequirements{Summary: "Butuh CRM 10 user"}})
	if !tr.EndRun || *tr.RunResult != domain.PlaybookResultQualified || tr.Requirements.Summary != "Butuh CRM 10 user" || tr.Next != nil {
		t.Fatalf("%+v", tr)
	}
}

func TestResolveNotFitNeedsValidReason(t *testing.T) {
	pb := defaultPlaybook()
	_, err := Resolve(ResolveRequest{Playbook: pb, Step: pb.Steps[1], AttemptNo: 1, OutcomeKey: "not_fit",
		Input: OutcomeInput{Disqualify: &DisqualifyInput{Reason: "whatever"}}, Now: now})
	if !errors.Is(err, ErrOutcomeInputRequired) {
		t.Fatalf("invalid reason: err=%v", err)
	}
	tr := resolve(t, 1, 1, false, "not_fit", OutcomeInput{Disqualify: &DisqualifyInput{Reason: domain.DisqualifyBudget, Note: "mahal"}})
	if *tr.DisqualifyReason != domain.DisqualifyBudget || tr.DisqualifyNote != "mahal" || *tr.SetLeadStatus != domain.LeadStatusUnqualified {
		t.Fatalf("%+v", tr)
	}
}

func TestResolveRequiresKnownOutcome(t *testing.T) {
	pb := defaultPlaybook()
	if _, err := Resolve(ResolveRequest{Playbook: pb, Step: pb.Steps[0], AttemptNo: 1, OutcomeKey: "", Now: now}); !errors.Is(err, ErrOutcomeRequired) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := Resolve(ResolveRequest{Playbook: pb, Step: pb.Steps[0], AttemptNo: 1, OutcomeKey: "qualified", Now: now}); !errors.Is(err, ErrOutcomeInvalid) {
		t.Fatalf("other step's outcome: %v", err)
	}
}

func TestStepForStatus(t *testing.T) {
	pb := defaultPlaybook()
	for status, want := range map[domain.LeadStatus]string{
		domain.LeadStatusNew: "first_contact", domain.LeadStatusAttempting: "first_contact", domain.LeadStatusContacted: "discovery",
	} {
		step, ok := StepForStatus(pb, status)
		if !ok || step.Key != want {
			t.Fatalf("%s → %v %s", status, ok, step.Key)
		}
	}
	for _, status := range []domain.LeadStatus{domain.LeadStatusQualified, domain.LeadStatusUnqualified, domain.LeadStatusConverted} {
		if _, ok := StepForStatus(pb, status); ok {
			t.Fatalf("%s must have no step", status)
		}
	}
}
