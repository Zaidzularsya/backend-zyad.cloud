package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmservice "zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	"zyad.cloud/internal/platform/database"
)

const (
	metaBillingSuspended        = "billing_suspended"
	metaBillingSuspendedAt      = "billing_suspended_at"
	metaBillingSuspendedInvoice = "billing_suspended_invoice"

	eventWorkspaceSuspended   = "self_serve.workspace_suspended"
	eventWorkspaceReactivated = "self_serve.workspace_reactivated"
)

// workspaceOrganizations: pembacaan/penulisan organisasi yang dipakai adapter (OrganizationRepository).
type workspaceOrganizations interface {
	FindByID(ctx context.Context, id string) (model.Organization, error)
	Update(ctx context.Context, id string, params organizationrepo.UpdateOrganizationParams) (model.Organization, error)
}

type workspaceLifecycle interface {
	ChangeStatus(ctx context.Context, current model.Organization, input organizationservice.ChangeOrganizationStatusInput) (model.Organization, error)
}

// workspaceStatus mengimplementasikan crmservice.WorkspaceStatusPort.
type workspaceStatus struct {
	orgs      workspaceOrganizations
	lifecycle workspaceLifecycle
	now       func() time.Time
}

var _ crmservice.WorkspaceStatusPort = workspaceStatus{}

func newWorkspaceStatus(orgs workspaceOrganizations, lifecycle workspaceLifecycle) workspaceStatus {
	return workspaceStatus{orgs: orgs, lifecycle: lifecycle, now: time.Now}
}

func (w workspaceStatus) Get(ctx context.Context, organizationID string) (crmservice.WorkspaceState, error) {
	org, err := w.orgs.FindByID(ctx, organizationID)
	if err != nil {
		return crmservice.WorkspaceState{}, err
	}
	flag, _ := org.Metadata[metaBillingSuspended].(bool)
	return crmservice.WorkspaceState{ID: org.ID, Name: org.Name, Status: string(org.Status), BillingSuspended: flag}, nil
}

// SuspendForBilling mengubah status dulu, baru menulis penanda. Bila penanda gagal ditulis, workspace tetap
// suspended tanpa penanda sehingga job tidak akan membukanya otomatis; kondisi itu dicatat untuk penanganan manual.
func (w workspaceStatus) SuspendForBilling(ctx context.Context, organizationID, invoiceNumber, actorUserID string) error {
	current, err := w.orgs.FindByID(ctx, organizationID)
	if err != nil {
		return err
	}
	updated, err := w.lifecycle.ChangeStatus(ctx, current, organizationservice.ChangeOrganizationStatusInput{
		OrganizationID: organizationID, Status: coretenant.OrganizationStatusSuspended,
		Reason: "billing_overdue: " + invoiceNumber, ActorUserID: actorUserID,
	})
	if err != nil {
		return err
	}
	meta := cloneMetadata(updated.Metadata)
	meta[metaBillingSuspended] = true
	meta[metaBillingSuspendedAt] = w.now().UTC().Format(time.RFC3339)
	meta[metaBillingSuspendedInvoice] = invoiceNumber
	if _, err := w.orgs.Update(ctx, organizationID, organizationrepo.UpdateOrganizationParams{Metadata: &meta}); err != nil {
		slog.ErrorContext(ctx, "workspace suspended for billing but metadata write failed; needs manual follow-up",
			"organization_id", organizationID, "invoice_number", invoiceNumber, "error", err)
		return fmt.Errorf("write billing suspend metadata: %w", err)
	}
	return nil
}

func (w workspaceStatus) ReactivateFromBilling(ctx context.Context, organizationID, invoiceNumber, actorUserID string) error {
	current, err := w.orgs.FindByID(ctx, organizationID)
	if err != nil {
		return err
	}
	updated, err := w.lifecycle.ChangeStatus(ctx, current, organizationservice.ChangeOrganizationStatusInput{
		OrganizationID: organizationID, Status: coretenant.OrganizationStatusActive,
		Reason: "billing_paid: " + invoiceNumber, ActorUserID: actorUserID,
	})
	if err != nil {
		return err
	}
	meta := cloneMetadata(updated.Metadata)
	delete(meta, metaBillingSuspended)
	delete(meta, metaBillingSuspendedAt)
	delete(meta, metaBillingSuspendedInvoice)
	if _, err := w.orgs.Update(ctx, organizationID, organizationrepo.UpdateOrganizationParams{Metadata: &meta}); err != nil {
		slog.ErrorContext(ctx, "workspace reactivated but billing metadata not cleared",
			"organization_id", organizationID, "invoice_number", invoiceNumber, "error", err)
		return fmt.Errorf("clear billing suspend metadata: %w", err)
	}
	return nil
}

func cloneMetadata(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+3)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// workspaceOwners mendaftar owner aktif workspace (role organization_owner).
type workspaceOwners interface {
	ListOwners(ctx context.Context, organizationID string) ([]crmdomain.OrganizationMember, error)
}

type workspaceOwnerRepo struct{ db *database.Pool }

func (r workspaceOwnerRepo) ListOwners(ctx context.Context, organizationID string) ([]crmdomain.OrganizationMember, error) {
	// organization_memberships, user_roles, dan users tidak di bawah RLS: filter organisasi eksplisit.
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT u.id, u.name, u.email
		FROM organization_memberships m
		JOIN users u ON u.id = m.user_id
		JOIN user_roles ur ON ur.user_id = u.id AND (ur.organization_id = m.organization_id OR ur.organization_id IS NULL)
		JOIN roles r ON r.id = ur.role_id
		WHERE m.organization_id = $1 AND m.status = 'active' AND u.deleted_at IS NULL
			AND (r.slug = 'organization_owner' OR r.role_name = 'organization_owner')
			AND COALESCE(u.email, '') <> ''
		ORDER BY u.email`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []crmdomain.OrganizationMember
	for rows.Next() {
		var m crmdomain.OrganizationMember
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type accessEventPublisher interface {
	Publish(ctx context.Context, event notificationpublisher.Event) (notificationdomain.OutboxEvent, error)
}

// accessNotifier mengimplementasikan crmservice.AccessNotifier: satu event per owner.
type accessNotifier struct {
	owners    workspaceOwners
	orgs      workspaceOrganizations
	publisher accessEventPublisher
	appName   string
	locale    string
}

var _ crmservice.AccessNotifier = accessNotifier{}

func (n accessNotifier) WorkspaceSuspended(ctx context.Context, organizationID string, p crmservice.SuspendNotice) error {
	return n.publish(ctx, organizationID, eventWorkspaceSuspended, func(m crmdomain.OrganizationMember) map[string]any {
		return map[string]any{
			"app_name": n.appName, "user_name": m.Name, "workspace_name": p.WorkspaceName,
			"invoice_number": p.InvoiceNumber, "invoice_url": p.InvoiceURL, "amount_due": p.AmountDue,
			"due_date": p.DueDate.Format("2006-01-02"), "grace_days": fmt.Sprint(p.GraceDays),
		}
	})
}

func (n accessNotifier) WorkspaceReactivated(ctx context.Context, organizationID string, invoiceNumber string) error {
	org, err := n.orgs.FindByID(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("read workspace: %w", err)
	}
	return n.publish(ctx, organizationID, eventWorkspaceReactivated, func(m crmdomain.OrganizationMember) map[string]any {
		return map[string]any{"app_name": n.appName, "user_name": m.Name, "workspace_name": org.Name, "invoice_number": invoiceNumber}
	})
}

func (n accessNotifier) publish(ctx context.Context, organizationID, eventType string, payload func(crmdomain.OrganizationMember) map[string]any) error {
	owners, err := n.owners.ListOwners(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("list workspace owners: %w", err)
	}
	if len(owners) == 0 {
		return errors.New("workspace has no active owner with email")
	}
	var errs []error
	for _, m := range owners {
		_, err := n.publisher.Publish(ctx, notificationpublisher.Event{
			Type: eventType, OrganizationID: organizationID, UserID: m.UserID,
			Recipient: notificationdomain.NotificationRecipient{Type: "user", UserID: m.UserID, Name: m.Name, Email: m.Email},
			Payload:   payload(m), Locale: n.locale,
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
