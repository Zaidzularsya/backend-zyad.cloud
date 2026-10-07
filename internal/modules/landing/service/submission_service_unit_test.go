package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// ── stubs ──────────────────────────────────────────────────────────────────

type syncSubmissionRepoStub struct {
	repository.SubmissionRepository

	stored      map[string]domain.LandingSubmission
	byKey       map[string]string
	createCalls int
	createErr   error
	updates     []repository.UpdateCRMSyncParams
	updateErr   error
}

func newSyncSubmissionRepoStub() *syncSubmissionRepoStub {
	return &syncSubmissionRepoStub{stored: map[string]domain.LandingSubmission{}, byKey: map[string]string{}}
}

func (s *syncSubmissionRepoStub) Create(_ context.Context, _ coretenant.Scope, p repository.CreateSubmissionParams) (domain.LandingSubmission, error) {
	s.createCalls++
	if s.createErr != nil {
		return domain.LandingSubmission{}, s.createErr
	}
	sub := domain.LandingSubmission{
		ID:             "sub-1",
		LandingPageID:  p.LandingPageID,
		FormID:         p.FormID,
		Status:         p.Status,
		SubmittedData:  p.SubmittedData,
		IdempotencyKey: p.IdempotencyKey,
		CRMSyncStatus:  domain.CRMSyncSkipped,
	}
	s.stored[sub.ID] = sub
	s.byKey[p.IdempotencyKey] = sub.ID
	return sub, nil
}

func (s *syncSubmissionRepoStub) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.LandingSubmission, error) {
	sub, ok := s.stored[id]
	if !ok {
		return domain.LandingSubmission{}, pgx.ErrNoRows
	}
	return sub, nil
}

func (s *syncSubmissionRepoStub) FindByIdempotencyKey(_ context.Context, _ coretenant.Scope, key string) (domain.LandingSubmission, error) {
	id, ok := s.byKey[key]
	if !ok {
		return domain.LandingSubmission{}, pgx.ErrNoRows
	}
	return s.stored[id], nil
}

func (s *syncSubmissionRepoStub) UpdateCRMSync(_ context.Context, _ coretenant.Scope, id string, p repository.UpdateCRMSyncParams) (domain.LandingSubmission, error) {
	if s.updateErr != nil {
		return domain.LandingSubmission{}, s.updateErr
	}
	s.updates = append(s.updates, p)
	sub := s.stored[id]
	sub.CRMLeadID = p.LeadID
	sub.CRMSyncStatus = p.Status
	sub.CRMSyncError = p.Error
	s.stored[id] = sub
	return sub, nil
}

type syncFormRepoStub struct {
	repository.FormRepository
	form   domain.LandingForm
	fields []domain.LandingFormField
}

func (s *syncFormRepoStub) FindByID(context.Context, coretenant.Scope, string) (domain.LandingForm, error) {
	return s.form, nil
}

func (s *syncFormRepoStub) ListFieldsByForm(context.Context, coretenant.Scope, string) ([]domain.LandingFormField, error) {
	return s.fields, nil
}

type syncPageRepoStub struct {
	repository.PageRepository
	page domain.LandingPage
}

func (s *syncPageRepoStub) FindByID(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return s.page, nil
}

type leadSinkStub struct {
	calls  int
	input  LeadSinkInput
	result LeadSinkResult
	err    error
}

func (s *leadSinkStub) CreateFromSubmission(_ context.Context, _ coretenant.Scope, in LeadSinkInput) (LeadSinkResult, error) {
	s.calls++
	s.input = in
	return s.result, s.err
}

type featureCheckerStub struct {
	err   error
	calls int
	key   string
}

func (s *featureCheckerStub) RequireFeature(_ context.Context, _ string, key string) error {
	s.calls++
	s.key = key
	return s.err
}

type syncFixture struct {
	svc      SubmissionService
	subs     *syncSubmissionRepoStub
	forms    *syncFormRepoStub
	pages    *syncPageRepoStub
	sink     *leadSinkStub
	features *featureCheckerStub
}

func newSyncFixture() *syncFixture {
	f := &syncFixture{
		subs: newSyncSubmissionRepoStub(),
		forms: &syncFormRepoStub{
			form: domain.LandingForm{
				ID: "form-1", LandingPageID: "page-1", IsActive: true,
				CreateCRMLead: true, LeadOwnerUserID: "owner-1",
			},
			fields: standardTestForm().Fields,
		},
		pages:    &syncPageRepoStub{page: domain.LandingPage{ID: "page-1", CreatedBy: "creator-1"}},
		sink:     &leadSinkStub{result: LeadSinkResult{LeadID: "lead-1"}},
		features: &featureCheckerStub{},
	}
	f.svc = NewSubmissionService(f.subs, f.forms, f.pages, f.sink, f.features)
	return f
}

func submitParams(key string) repository.CreateSubmissionParams {
	return repository.CreateSubmissionParams{
		FormID:         "form-1",
		IdempotencyKey: key,
		SubmittedData:  map[string]any{"name": "Budi", "email": "budi@x.id", "message": "Halo"},
	}
}

// ── tests ──────────────────────────────────────────────────────────────────

func TestSubmitForm_PlatformCreatesLead(t *testing.T) {
	f := newSyncFixture()

	sub, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k1"), true)
	if err != nil {
		t.Fatalf("SubmitForm() error = %v", err)
	}

	if f.sink.calls != 1 {
		t.Fatalf("sink calls = %d, want 1", f.sink.calls)
	}
	if f.features.calls != 0 {
		t.Fatalf("platform org must not consult feature checker, calls = %d", f.features.calls)
	}
	if f.sink.input.OwnerUserID != "owner-1" || f.sink.input.Email != "budi@x.id" || f.sink.input.SubmissionID != "sub-1" {
		t.Fatalf("unexpected sink input: %+v", f.sink.input)
	}
	if len(f.subs.updates) != 1 || f.subs.updates[0].Status != domain.CRMSyncCreated || f.subs.updates[0].LeadID != "lead-1" {
		t.Fatalf("updates = %+v, want one created/lead-1", f.subs.updates)
	}
	if sub.CRMSyncStatus != domain.CRMSyncCreated || sub.CRMLeadID != "lead-1" {
		t.Fatalf("returned submission = %+v", sub)
	}
}

func TestSubmitForm_MergedResultIsRecorded(t *testing.T) {
	f := newSyncFixture()
	f.sink.result = LeadSinkResult{LeadID: "lead-9", Merged: true}

	sub, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k1"), true)
	if err != nil {
		t.Fatalf("SubmitForm() error = %v", err)
	}
	if sub.CRMSyncStatus != domain.CRMSyncMerged || sub.CRMLeadID != "lead-9" {
		t.Fatalf("returned submission = %+v", sub)
	}
}

func TestSubmitForm_TenantWithFeatureCreatesLead(t *testing.T) {
	f := newSyncFixture()

	if _, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k1"), false); err != nil {
		t.Fatalf("SubmitForm() error = %v", err)
	}
	if f.features.key != FeatureCRMLeadForm || f.sink.calls != 1 {
		t.Fatalf("feature key = %q, sink calls = %d", f.features.key, f.sink.calls)
	}
}

func TestSubmitForm_TenantWithoutFeatureSkips(t *testing.T) {
	f := newSyncFixture()
	f.features.err = errors.New("feature not entitled")

	sub, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k1"), false)
	if err != nil {
		t.Fatalf("SubmitForm() error = %v", err)
	}
	if f.sink.calls != 0 {
		t.Fatalf("sink must not be called, calls = %d", f.sink.calls)
	}
	if sub.CRMSyncStatus != domain.CRMSyncSkipped || len(f.subs.updates) != 0 {
		t.Fatalf("status = %q, updates = %+v", sub.CRMSyncStatus, f.subs.updates)
	}
}

func TestSubmitForm_FormFlagOffSkips(t *testing.T) {
	f := newSyncFixture()
	f.forms.form.CreateCRMLead = false

	sub, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k1"), true)
	if err != nil {
		t.Fatalf("SubmitForm() error = %v", err)
	}
	if f.sink.calls != 0 || sub.CRMSyncStatus != domain.CRMSyncSkipped {
		t.Fatalf("sink calls = %d, status = %q", f.sink.calls, sub.CRMSyncStatus)
	}
}

func TestSubmitForm_NilSinkOrFeaturesSkips(t *testing.T) {
	f := newSyncFixture()
	svc := NewSubmissionService(f.subs, f.forms, f.pages, nil, nil)

	sub, err := svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k1"), true)
	if err != nil {
		t.Fatalf("SubmitForm() error = %v", err)
	}
	if sub.CRMSyncStatus != domain.CRMSyncSkipped {
		t.Fatalf("status = %q, want skipped", sub.CRMSyncStatus)
	}

	// Tenant tanpa FeatureChecker juga skipped (tidak boleh fail-open).
	svc = NewSubmissionService(f.subs, f.forms, f.pages, f.sink, nil)
	sub, err = svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k2"), false)
	if err != nil || sub.CRMSyncStatus != domain.CRMSyncSkipped || f.sink.calls != 0 {
		t.Fatalf("err=%v status=%q sinkCalls=%d", err, sub.CRMSyncStatus, f.sink.calls)
	}
}

func TestSubmitForm_SinkErrorStillReturnsSubmission(t *testing.T) {
	f := newSyncFixture()
	f.sink.err = errors.New("crm down")

	sub, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k1"), true)
	if err != nil {
		t.Fatalf("SubmitForm() must not return sync error, got %v", err)
	}
	if sub.ID != "sub-1" {
		t.Fatalf("submission must still be returned, got %+v", sub)
	}
	if len(f.subs.updates) != 1 || f.subs.updates[0].Status != domain.CRMSyncFailed || f.subs.updates[0].Error != "crm down" {
		t.Fatalf("updates = %+v", f.subs.updates)
	}
}

func TestSubmitForm_OwnerMissingRecordsFailed(t *testing.T) {
	f := newSyncFixture()
	f.forms.form.LeadOwnerUserID = ""
	f.pages.page.CreatedBy = ""

	if _, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k1"), true); err != nil {
		t.Fatalf("SubmitForm() error = %v", err)
	}
	if f.sink.calls != 0 {
		t.Fatalf("sink must not be called without owner")
	}
	want := "LANDING_FORM_OWNER_MISSING: PIC lead belum diatur"
	if len(f.subs.updates) != 1 || f.subs.updates[0].Status != domain.CRMSyncFailed || f.subs.updates[0].Error != want {
		t.Fatalf("updates = %+v", f.subs.updates)
	}
}

func TestSubmitForm_InactiveFormReturnsErrFormNotActive(t *testing.T) {
	f := newSyncFixture()
	f.forms.form.IsActive = false

	_, err := f.svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("k1"), true)
	if !errors.Is(err, ErrFormNotActive) {
		t.Fatalf("error = %v, want ErrFormNotActive", err)
	}
	if f.sink.calls != 0 || f.subs.createCalls != 0 {
		t.Fatalf("sink calls = %d, create calls = %d, want 0/0", f.sink.calls, f.subs.createCalls)
	}
}

func TestSubmitForm_DuplicateIdempotencyKeyReturnsExistingWithoutResync(t *testing.T) {
	f := newSyncFixture()
	scope := mustLandingScope(t)

	first, err := f.svc.SubmitForm(context.Background(), scope, submitParams("same"), true)
	if err != nil {
		t.Fatalf("first SubmitForm() error = %v", err)
	}
	second, err := f.svc.SubmitForm(context.Background(), scope, submitParams("same"), true)
	if err != nil {
		t.Fatalf("second SubmitForm() error = %v", err)
	}

	if second.ID != first.ID || second.CRMLeadID != "lead-1" {
		t.Fatalf("second = %+v, want the stored submission with lead", second)
	}
	if f.subs.createCalls != 1 || f.sink.calls != 1 {
		t.Fatalf("create calls = %d, sink calls = %d, want 1/1", f.subs.createCalls, f.sink.calls)
	}
}

func TestSubmitForm_IdempotencyRaceOnInsertReturnsExisting(t *testing.T) {
	f := newSyncFixture()
	// Simulasi: dua request lolos pencarian awal, insert kedua kena unique violation.
	f.subs.stored["sub-0"] = domain.LandingSubmission{ID: "sub-0", CRMLeadID: "lead-0", CRMSyncStatus: domain.CRMSyncCreated}
	f.subs.createErr = &pgconn.PgError{Code: "23505"}
	lookups := 0
	svc := NewSubmissionService(&raceSubmissionRepo{syncSubmissionRepoStub: f.subs, lookups: &lookups}, f.forms, f.pages, f.sink, f.features)

	sub, err := svc.SubmitForm(context.Background(), mustLandingScope(t), submitParams("race"), true)
	if err != nil {
		t.Fatalf("SubmitForm() error = %v", err)
	}
	if sub.ID != "sub-0" || f.sink.calls != 0 {
		t.Fatalf("sub = %+v, sink calls = %d", sub, f.sink.calls)
	}
}

// raceSubmissionRepo menyembunyikan baris lama pada pencarian pertama saja.
type raceSubmissionRepo struct {
	*syncSubmissionRepoStub
	lookups *int
}

func (r *raceSubmissionRepo) FindByIdempotencyKey(ctx context.Context, scope coretenant.Scope, key string) (domain.LandingSubmission, error) {
	*r.lookups++
	if *r.lookups == 1 {
		return domain.LandingSubmission{}, pgx.ErrNoRows
	}
	return r.stored["sub-0"], nil
}

func TestRetryCRMSync_IdempotentWhenLeadExists(t *testing.T) {
	f := newSyncFixture()
	f.subs.stored["sub-1"] = domain.LandingSubmission{
		ID: "sub-1", FormID: "form-1", LandingPageID: "page-1",
		CRMLeadID: "lead-1", CRMSyncStatus: domain.CRMSyncCreated,
	}

	sub, err := f.svc.RetryCRMSync(context.Background(), mustLandingScope(t), "sub-1", true)
	if err != nil {
		t.Fatalf("RetryCRMSync() error = %v", err)
	}
	if f.sink.calls != 0 || len(f.subs.updates) != 0 {
		t.Fatalf("sink calls = %d, updates = %+v, want none", f.sink.calls, f.subs.updates)
	}
	if sub.CRMLeadID != "lead-1" {
		t.Fatalf("sub = %+v", sub)
	}
}

func TestRetryCRMSync_AfterOwnerFixed(t *testing.T) {
	f := newSyncFixture()
	f.forms.form.LeadOwnerUserID = ""
	f.pages.page.CreatedBy = ""
	scope := mustLandingScope(t)

	first, err := f.svc.SubmitForm(context.Background(), scope, submitParams("k1"), true)
	if err != nil || first.CRMSyncStatus != domain.CRMSyncFailed {
		t.Fatalf("first: err=%v status=%q", err, first.CRMSyncStatus)
	}

	f.forms.form.LeadOwnerUserID = "owner-2"
	retried, err := f.svc.RetryCRMSync(context.Background(), scope, first.ID, true)
	if err != nil {
		t.Fatalf("RetryCRMSync() error = %v", err)
	}
	if retried.CRMSyncStatus != domain.CRMSyncCreated || retried.CRMLeadID != "lead-1" || retried.CRMSyncError != "" {
		t.Fatalf("retried = %+v", retried)
	}
	if f.sink.input.OwnerUserID != "owner-2" {
		t.Fatalf("sink owner = %q, want owner-2", f.sink.input.OwnerUserID)
	}
}

func TestRetryCRMSync_NotFound(t *testing.T) {
	f := newSyncFixture()

	_, err := f.svc.RetryCRMSync(context.Background(), mustLandingScope(t), "missing", true)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("error = %v, want pgx.ErrNoRows", err)
	}
	if f.sink.calls != 0 {
		t.Fatalf("sink must not be called")
	}
}
