package template

import (
	"sort"

	"zyad.cloud/internal/core/notification/domain"
)

type VariableRegistry struct {
	variables map[string][]domain.NotificationVariable
}

func NewVariableRegistry() *VariableRegistry {
	return &VariableRegistry{
		variables: defaultVariables(),
	}
}

func (r *VariableRegistry) Get(templateCode string) []domain.NotificationVariable {
	variables := r.variables[templateCode]
	if len(variables) == 0 {
		return nil
	}

	result := make([]domain.NotificationVariable, len(variables))
	copy(result, variables)
	return result
}

func (r *VariableRegistry) Has(templateCode string, key string) bool {
	for _, variable := range r.variables[templateCode] {
		if variable.Key == key {
			return true
		}
	}
	return false
}

func (r *VariableRegistry) Codes() []string {
	codes := make([]string, 0, len(r.variables))
	for code := range r.variables {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func defaultVariables() map[string][]domain.NotificationVariable {
	return map[string][]domain.NotificationVariable{
		"auth.password_reset": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Recipient user name", Required: true, Example: "Admin"},
			{Key: "reset_url", Description: "Password reset URL", Required: true, Example: "https://app.example.test/reset"},
			{Key: "expired_at", Description: "Reset link expiration time", Required: true, Example: "2026-06-12 10:00 WIB"},
		},
		"auth.password_changed": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Recipient user name", Required: true, Example: "Admin"},
			{Key: "changed_at", Description: "Password change timestamp", Required: true, Example: "2026-06-12 10:00 WIB"},
		},
		"user.invitation": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "inviter_name", Description: "User who sent the invitation", Required: true, Example: "Owner"},
			{Key: "invitee_email", Description: "Invited user email", Required: true, Example: "member@example.test"},
			{Key: "organization_name", Description: "Organization name", Required: true, Example: "Acme Network"},
			{Key: "invitation_url", Description: "Invitation accept URL", Required: true, Example: "https://app.example.test/invitations/accept"},
			{Key: "expired_at", Description: "Invitation expiration time", Required: true, Example: "2026-06-19 10:00 WIB"},
		},
		"lead.created": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "lead_name", Description: "Lead name", Required: true, Example: "Budi"},
			{Key: "lead_source", Description: "Lead source", Required: true, Example: "landing_page"},
			{Key: "lead_phone", Description: "Lead phone number", Required: false, Example: "+628123456789"},
			{Key: "assigned_sales_name", Description: "Assigned sales name", Required: false, Example: "Sales Team"},
			{Key: "crm_url", Description: "CRM lead detail URL", Required: true, Example: "https://app.example.test/admin/leads/123"},
		},
		"payment.paid": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "customer_name", Description: "Customer name", Required: true, Example: "Budi"},
			{Key: "invoice_number", Description: "Invoice number", Required: true, Example: "INV-2026-0001"},
			{Key: "amount", Description: "Paid amount", Required: true, Example: "Rp150.000"},
			{Key: "paid_at", Description: "Payment timestamp", Required: true, Example: "2026-06-12 10:00 WIB"},
		},
		"security.new_login": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Recipient user name", Required: true, Example: "Admin"},
			{Key: "login_at", Description: "Login timestamp", Required: true, Example: "2026-06-12 10:00 WIB"},
			{Key: "ip_address", Description: "Login IP address", Required: true, Example: "127.0.0.1"},
			{Key: "device_name", Description: "Device or browser name", Required: false, Example: "Chrome on Linux"},
		},
		"crm.quotation_approved": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Recipient user name", Required: true, Example: "Sari"},
			{Key: "quotation_number", Description: "Quotation number", Required: true, Example: "QUO-2026-0001"},
			{Key: "responder_name", Description: "Name the customer entered when responding", Required: true, Example: "Budi Santoso"},
			{Key: "deal_url", Description: "Deal (or quotation) page URL", Required: true, Example: "https://app.example.test/app/crm/deals/1"},
		},
		"crm.quotation_revision_requested": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Recipient user name", Required: true, Example: "Sari"},
			{Key: "quotation_number", Description: "Quotation number", Required: true, Example: "QUO-2026-0001"},
			{Key: "responder_name", Description: "Name the customer entered when responding", Required: true, Example: "Budi Santoso"},
			{Key: "categories", Description: "Requested revision categories (labels, comma separated)", Required: true, Example: "Harga/diskon, Lainnya"},
			{Key: "note", Description: "Customer note (plain text, optional)", Required: false, Example: "Mohon diskon tambahan"},
			{Key: "deal_url", Description: "Deal (or quotation) page URL", Required: true, Example: "https://app.example.test/app/crm/deals/1"},
		},
		"whatsapp.session_disconnected": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Recipient user name", Required: true, Example: "Admin"},
			{Key: "session_name", Description: "WhatsApp session label (name and number)", Required: true, Example: "Sales +6281234567890"},
			{Key: "status", Description: "Current session status", Required: true, Example: "gagal"},
			{Key: "occurred_at", Description: "When the disconnect was observed", Required: true, Example: "2026-09-24 10:00 WIB"},
			{Key: "connections_url", Description: "WhatsApp connections page URL", Required: true, Example: "https://app.example.test/app/whatsapp"},
		},
		"receivable.invoice_send_failed": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Recipient user name", Required: true, Example: "Sari"},
			{Key: "invoice_number", Description: "Invoice number", Required: true, Example: "INV-2026-0001"},
			{Key: "recipient", Description: "Intended recipient (email or phone)", Required: true, Example: "budi@example.test"},
			{Key: "channel", Description: "Channel label (Email or WhatsApp)", Required: true, Example: "Email"},
			{Key: "reason", Description: "Failure reason (no provider internals)", Required: true, Example: "Belum ada pengirim."},
			{Key: "invoice_url", Description: "Invoice page URL", Required: true, Example: "https://app.example.test/app/billing/invoices/1"},
		},
		"self_serve.workspace_suspended": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Recipient user name", Required: true, Example: "Sari"},
			{Key: "workspace_name", Description: "Workspace name", Required: true, Example: "Toko Maju"},
			{Key: "invoice_number", Description: "Overdue invoice number", Required: true, Example: "INV-2026-0377"},
			{Key: "invoice_url", Description: "Public invoice payment link", Required: true, Example: "https://app.example.test/p/abc"},
			{Key: "amount_due", Description: "Amount due (formatted)", Required: true, Example: "Rp333.000"},
			{Key: "due_date", Description: "Invoice due date", Required: true, Example: "2026-09-10"},
			{Key: "grace_days", Description: "Grace period in days", Required: true, Example: "7"},
		},
		"self_serve.workspace_reactivated": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Recipient user name", Required: true, Example: "Sari"},
			{Key: "workspace_name", Description: "Workspace name", Required: true, Example: "Toko Maju"},
			{Key: "invoice_number", Description: "Settled invoice number", Required: true, Example: "INV-2026-0377"},
		},
		"permission.updated": {
			{Key: "app_name", Description: "Application name", Required: true, Example: "Zyad Cloud"},
			{Key: "user_name", Description: "Affected user name", Required: true, Example: "Admin"},
			{Key: "actor_name", Description: "User who changed the permission", Required: true, Example: "Super Admin"},
			{Key: "permission_name", Description: "Permission name", Required: true, Example: "notification_template.update"},
			{Key: "updated_at", Description: "Permission update timestamp", Required: true, Example: "2026-06-12 10:00 WIB"},
		},
	}
}
