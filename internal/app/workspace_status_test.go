package app

import (
	"context"
	"errors"
	"testing"
	"time"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmservice "zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
)

type fakeOrgs struct {
	org       model.Organization
	updateErr error
	updates   int
}

func (f *fakeOrgs) FindByID(context.Context, string) (model.Organization, error) { return f.org, nil }
func (f *fakeOrgs) Update(_ context.Context, _ string, p organizationrepo.UpdateOrganizationParams) (model.Organization, error) {
	f.updates++
	if f.updateErr != nil {
		return model.Organization{}, f.updateErr
	}
	f.org.Metadata = *p.Metadata
	return f.org, nil
}

type fakeLifecycle struct {
	orgs  *fakeOrgs
	calls []organizationservice.ChangeOrganizationStatusInput
}

func (f *fakeLifecycle) ChangeStatus(_ context.Context, cur model.Organization, in organizationservice.ChangeOrganizationStatusInput) (model.Organization, error) {
	f.calls = append(f.calls, in)
	f.orgs.org.Status = in.Status
	return f.orgs.org, nil
}

func newStatusFixture(meta map[string]any) (workspaceStatus, *fakeOrgs, *fakeLifecycle) {
	orgs := &fakeOrgs{org: model.Organization{ID: "w1", Name: "Toko Maju", Status: coretenant.OrganizationStatusActive, Metadata: meta}}
	lc := &fakeLifecycle{orgs: orgs}
	ws := newWorkspaceStatus(orgs, lc)
	ws.now = func() time.Time { return time.Date(2026, 10, 7, 3, 0, 0, 0, time.UTC) }
	return ws, orgs, lc
}

func TestSuspendForBillingSetsStatusAndMetadata(t *testing.T) {
	ws, orgs, lc := newStatusFixture(map[string]any{"other": "keep"})
	if err := ws.SuspendForBilling(context.Background(), "w1", "INV-2026-0377", "bot"); err != nil {
		t.Fatal(err)
	}
	if len(lc.calls) != 1 || lc.calls[0].Status != coretenant.OrganizationStatusSuspended ||
		lc.calls[0].Reason != "billing_overdue: INV-2026-0377" || lc.calls[0].ActorUserID != "bot" {
		t.Fatalf("calls = %+v", lc.calls)
	}
	m := orgs.org.Metadata
	if m["billing_suspended"] != true || m["billing_suspended_at"] != "2026-10-07T03:00:00Z" ||
		m["billing_suspended_invoice"] != "INV-2026-0377" || m["other"] != "keep" {
		t.Fatalf("metadata = %+v", m)
	}
}

func TestSuspendForBillingMetadataFailureReturnsError(t *testing.T) {
	ws, orgs, lc := newStatusFixture(nil)
	orgs.updateErr = errors.New("db down")
	if err := ws.SuspendForBilling(context.Background(), "w1", "INV-1", "bot"); err == nil {
		t.Fatal("want error")
	}
	if len(lc.calls) != 1 {
		t.Fatalf("status must change first, calls = %d", len(lc.calls))
	}
}

func TestReactivateFromBillingClearsOnlyBillingKeys(t *testing.T) {
	ws, orgs, lc := newStatusFixture(map[string]any{
		"billing_suspended": true, "billing_suspended_at": "x", "billing_suspended_invoice": "INV-1", "other": "keep",
	})
	orgs.org.Status = coretenant.OrganizationStatusSuspended
	if err := ws.ReactivateFromBilling(context.Background(), "w1", "INV-1", "bot"); err != nil {
		t.Fatal(err)
	}
	if lc.calls[0].Status != coretenant.OrganizationStatusActive || lc.calls[0].Reason != "billing_paid: INV-1" {
		t.Fatalf("calls = %+v", lc.calls)
	}
	if len(orgs.org.Metadata) != 1 || orgs.org.Metadata["other"] != "keep" {
		t.Fatalf("metadata = %+v", orgs.org.Metadata)
	}
}

func TestGetReadsBillingSuspendedFromMetadata(t *testing.T) {
	ws, _, _ := newStatusFixture(map[string]any{"billing_suspended": true})
	st, err := ws.Get(context.Background(), "w1")
	if err != nil || !st.BillingSuspended || st.Status != "active" || st.Name != "Toko Maju" {
		t.Fatalf("state = %+v err=%v", st, err)
	}
	ws, _, _ = newStatusFixture(nil)
	if st, _ := ws.Get(context.Background(), "w1"); st.BillingSuspended {
		t.Fatal("no metadata must not be billing suspended")
	}
}

type fakeOwners struct {
	list []crmdomain.OrganizationMember
}

func (f fakeOwners) ListOwners(context.Context, string) ([]crmdomain.OrganizationMember, error) {
	return f.list, nil
}

type recordingPublisher struct{ events []notificationpublisher.Event }

func (r *recordingPublisher) Publish(_ context.Context, e notificationpublisher.Event) (notificationdomain.OutboxEvent, error) {
	r.events = append(r.events, e)
	return notificationdomain.OutboxEvent{}, nil
}

func TestAccessNotifierPublishesPerOwner(t *testing.T) {
	pub := &recordingPublisher{}
	orgs := &fakeOrgs{org: model.Organization{ID: "w1", Name: "Toko Maju"}}
	n := accessNotifier{
		owners: fakeOwners{list: []crmdomain.OrganizationMember{{UserID: "u1", Name: "Sari", Email: "s@x.id"}, {UserID: "u2", Name: "Budi", Email: "b@x.id"}}},
		orgs:   orgs, publisher: pub, appName: "Zyad Cloud", locale: "id-ID",
	}
	err := n.WorkspaceSuspended(context.Background(), "w1", crmservice.SuspendNotice{
		WorkspaceName: "Toko Maju", InvoiceNumber: "INV-1", InvoiceURL: "https://x/p/1", AmountDue: "Rp333.000",
		DueDate: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), GraceDays: 7,
	})
	if err != nil || len(pub.events) != 2 {
		t.Fatalf("events = %d err=%v", len(pub.events), err)
	}
	e := pub.events[0]
	if e.Type != "self_serve.workspace_suspended" || e.OrganizationID != "w1" || e.Recipient.Email != "s@x.id" || e.Locale != "id-ID" ||
		e.Payload["invoice_url"] != "https://x/p/1" || e.Payload["due_date"] != "2026-09-10" || e.Payload["grace_days"] != "7" ||
		e.Payload["workspace_name"] != "Toko Maju" || e.Payload["user_name"] != "Sari" || e.Payload["amount_due"] != "Rp333.000" {
		t.Fatalf("event = %+v", e)
	}
	pub.events = nil
	if err := n.WorkspaceReactivated(context.Background(), "w1", "INV-1"); err != nil || len(pub.events) != 2 ||
		pub.events[1].Type != "self_serve.workspace_reactivated" || pub.events[1].Payload["workspace_name"] != "Toko Maju" {
		t.Fatalf("reactivated = %+v err=%v", pub.events, err)
	}
}
