package app

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/middleware"
	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	assetservice "zyad.cloud/internal/modules/asset/service"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	mailboxservice "zyad.cloud/internal/modules/mailbox/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	receivablehandler "zyad.cloud/internal/modules/receivable/handler"
	receivablerepo "zyad.cloud/internal/modules/receivable/repository"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	whatsappservice "zyad.cloud/internal/modules/whatsapp/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/shared/docpdf"
	"zyad.cloud/internal/shared/publiclink"
)

// receivableIssuer memakai sumber identitas penerbit yang sama dengan PDF penawaran
// (branding default, fallback nama organisasi).
type receivableIssuer struct {
	repo crmrepo.QuotationIssuerRepository
}

func (a receivableIssuer) OrganizationProfile(ctx context.Context, scope coretenant.Scope) (docpdf.Party, error) {
	issuer, err := a.repo.Find(ctx, scope)
	if err != nil {
		return docpdf.Party{}, err
	}
	return docpdf.Party{Name: issuer.Name, Email: issuer.Email, Phone: issuer.Phone, Address: issuer.Address}, nil
}

// receivableMembers: anggota aktif organisasi (read-only, dari repository member CRM yang hanya membaca
// organization_memberships) dan pemegang permission lewat checker RBAC yang sama dengan route.
type receivableMembers struct {
	members crmrepo.MemberRepository
	perms   permissionmiddleware.CombinedPermissionChecker
}

func (m receivableMembers) Active(ctx context.Context, scope coretenant.Scope) ([]receivableservice.Member, error) {
	list, err := m.members.ListActive(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make([]receivableservice.Member, 0, len(list))
	for _, member := range list {
		out = append(out, receivableservice.Member{UserID: member.UserID, Name: member.Name, Email: member.Email})
	}
	return out, nil
}

func (m receivableMembers) Find(ctx context.Context, scope coretenant.Scope, userID string) (receivableservice.Member, error) {
	list, err := m.Active(ctx, scope)
	if err != nil {
		return receivableservice.Member{}, err
	}
	for _, member := range list {
		if member.UserID == userID {
			return member, nil
		}
	}
	return receivableservice.Member{}, pgx.ErrNoRows
}

func (m receivableMembers) WithPermission(ctx context.Context, scope coretenant.Scope, permission string) ([]receivableservice.Member, error) {
	list, err := m.Active(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make([]receivableservice.Member, 0, len(list))
	for _, member := range list {
		if m.perms == nil || m.perms.CanOrganization(ctx, member.UserID, scope.OrganizationID(), []string{permission}) == nil {
			out = append(out, member)
		}
	}
	return out, nil
}

type notificationPublisher interface {
	Publish(ctx context.Context, event notificationpublisher.Event) (notificationdomain.OutboxEvent, error)
}

// receivableNotifier mengubah kegagalan kirim menjadi event outbox receivable.invoice_send_failed.
type receivableNotifier struct {
	publisher   notificationPublisher
	appName     string
	frontendURL string
	locale      string
}

func (n receivableNotifier) NotifySendFailed(ctx context.Context, scope coretenant.Scope, e receivableservice.FailureEvent) {
	_, err := n.publisher.Publish(ctx, notificationpublisher.Event{
		Type: "receivable.invoice_send_failed", OrganizationID: scope.OrganizationID(), UserID: e.UserID,
		Recipient: notificationdomain.NotificationRecipient{Type: "user", UserID: e.UserID, Name: e.UserName, Email: e.UserEmail},
		Payload: map[string]any{
			"app_name": n.appName, "user_name": e.UserName, "invoice_number": e.InvoiceNumber,
			"recipient": singleLine(e.Recipient), "channel": e.Channel, "reason": singleLine(e.Reason),
			"invoice_url": strings.TrimRight(n.frontendURL, "/") + "/app/billing/invoices/" + e.InvoiceID,
		},
		Locale: n.locale,
	})
	if err != nil {
		slog.WarnContext(ctx, "receivable: publish send-failed notification failed", "invoice_id", e.InvoiceID, "error", err)
	}
}

func singleLine(v string) string { return strings.Join(strings.Fields(v), " ") }

// receivableOrgLister memberi id organisasi aktif untuk job overdue.
type receivableOrgLister struct {
	repo *organizationrepo.OrganizationRepository
}

func (l receivableOrgLister) ListActive(ctx context.Context, limit, offset int) ([]string, error) {
	orgs, _, err := l.repo.List(ctx, organizationrepo.OrganizationListFilter{
		Status: coretenant.OrganizationStatusActive, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(orgs))
	for _, o := range orgs {
		ids = append(ids, o.ID)
	}
	return ids, nil
}

// lazyInvoiceDocs memutus ketergantungan melingkar: InvoiceService memakai SendService (kirim otomatis
// setelah terbit) dan SendService memakai InvoiceService (PDF + link).
type lazyInvoiceDocs struct {
	svc *receivableservice.InvoiceService
}

func (l lazyInvoiceDocs) PDF(ctx context.Context, scope coretenant.Scope, id string) (receivableservice.InvoicePDF, error) {
	return (*l.svc).PDF(ctx, scope, id)
}

func (l lazyInvoiceDocs) Link(ctx context.Context, scope coretenant.Scope, id, userID string) (string, time.Time, error) {
	return (*l.svc).Link(ctx, scope, id, userID)
}

type receivableBuild struct {
	DB                 *database.Pool
	Assets             assetservice.AssetService
	Links              *publiclink.Service
	Mailboxes          *mailboxservice.MailboxService
	Messages           *mailboxservice.MessageService
	WhatsApp           *whatsappservice.ConversationService
	Entitlements       middleware.EntitlementChecker
	Permissions        permissionmiddleware.CombinedPermissionChecker
	Publisher          notificationPublisher
	Members            crmrepo.MemberRepository
	Issuers            crmrepo.QuotationIssuerRepository
	AppName            string
	FrontendURL        string
	NotificationLocale string
	RateCounter        middleware.RateCounter
}

// receivableModule menampung handler untuk router dan service yang dibagikan ke modul lain
// (S4: SalesOrder membuat invoice lewat Invoices/Accounts dan mendaftar Listeners).
type receivableModule struct {
	AccountHandler  *receivablehandler.AccountHandler
	InvoiceHandler  *receivablehandler.InvoiceHandler
	PaymentHandler  *receivablehandler.PaymentHandler
	SettingsHandler *receivablehandler.SettingsHandler
	PublicHandler   *receivablehandler.PublicInvoiceHandler
	ContractHandler *receivablehandler.ContractHandler

	Invoices  receivableservice.InvoiceService
	Contracts receivableservice.ContractService
	Billing   receivableservice.OrderBilling
	Accounts  receivableservice.AccountService
	Payments  receivableservice.PaymentService
	Listeners *receivableservice.Registry
}

func buildReceivable(b receivableBuild) receivableModule {
	db := b.DB
	accountRepo := receivablerepo.NewAccountRepository(db)
	invoiceRepo := receivablerepo.NewInvoiceRepository(db)
	paymentRepo := receivablerepo.NewPaymentRepository(db)
	sendRepo := receivablerepo.NewSendRepository(db)
	settingsRepo := receivablerepo.NewSettingsRepository(db)
	counterRepo := receivablerepo.NewCounterRepository(db)
	contractRepo := receivablerepo.NewContractRepository(db)

	issuer := receivableIssuer{repo: b.Issuers}
	members := receivableMembers{members: b.Members, perms: b.Permissions}
	listeners := receivableservice.NewRegistry()

	var invoiceSvc receivableservice.InvoiceService
	sendSvc := receivableservice.NewSendService(receivableservice.SendDeps{
		Invoices: invoiceRepo, Sends: sendRepo, Settings: settingsRepo, Docs: lazyInvoiceDocs{svc: &invoiceSvc}, Issuers: issuer,
		// Kanal kirim: adapter di modul mailbox & whatsapp (receivable tidak mengimpor mereka).
		Email:        mailboxservice.NewInvoiceMailer(b.Mailboxes, b.Messages),
		WhatsApp:     whatsappservice.NewInvoiceSender(b.WhatsApp, b.Entitlements),
		Notifier:     receivableNotifier{publisher: b.Publisher, appName: b.AppName, frontendURL: b.FrontendURL, locale: b.NotificationLocale},
		Members:      members,
		Entitlements: b.Entitlements,
	})
	invoiceSvc = receivableservice.NewInvoiceService(receivableservice.InvoiceDeps{
		Accounts: accountRepo, Invoices: invoiceRepo, Counters: counterRepo, Settings: settingsRepo, Issuers: issuer,
		Files: b.Assets, Renderer: docpdf.NewRenderer(), Links: b.Links, Sender: sendSvc, Members: members,
		Listeners: listeners, FrontendURL: b.FrontendURL,
	})
	accountSvc := receivableservice.NewAccountService(accountRepo)
	paymentSvc := receivableservice.NewPaymentService(receivableservice.PaymentDeps{
		Payments: paymentRepo, Invoices: invoiceSvc, Listeners: listeners,
	})
	settingsSvc := receivableservice.NewSettingsService(settingsRepo, members)
	contractSvc := receivableservice.NewContractService(contractRepo, nil)
	orderBilling := receivableservice.NewOrderBilling(accountRepo, invoiceSvc, contractRepo, counterRepo, listeners)

	// Halaman publik: scope tenant dibangun dari baris link lewat resolver worker (identitas
	// public-document-link, sama dengan penawaran), lalu query berjalan di bawah RLS.
	publicSvc := receivableservice.NewPublicInvoiceService(
		b.Links,
		organizationservice.NewWorkerResolver(organizationrepo.NewOrganizationRepository(db), receivableservice.PublicLinkIdentity),
		invoiceSvc, issuer, b.Entitlements, nil,
	)

	return receivableModule{
		AccountHandler:  receivablehandler.NewAccountHandler(accountSvc),
		InvoiceHandler:  receivablehandler.NewInvoiceHandler(invoiceSvc, sendSvc),
		PaymentHandler:  receivablehandler.NewPaymentHandler(paymentSvc),
		SettingsHandler: receivablehandler.NewSettingsHandler(settingsSvc),
		PublicHandler:   receivablehandler.NewPublicInvoiceHandler(publicSvc, b.RateCounter),
		ContractHandler: receivablehandler.NewContractHandler(contractSvc),
		Invoices:        invoiceSvc, Accounts: accountSvc, Payments: paymentSvc, Listeners: listeners,
		Contracts: contractSvc, Billing: orderBilling,
	}
}

// NewReceivableWorkerResolver resolves organizations for the overdue job identity (cmd/worker only).
func NewReceivableWorkerResolver(db *database.Pool) *organizationservice.WorkerResolver {
	return organizationservice.NewWorkerResolver(organizationrepo.NewOrganizationRepository(db), receivableservice.OverdueWorkerIdentity)
}

// NewReceivableOverdueRunner builds the hourly overdue job (cmd/worker only).
func NewReceivableOverdueRunner(db *database.Pool) *receivableservice.OverdueRunner {
	return receivableservice.NewOverdueRunner(
		receivableOrgLister{repo: organizationrepo.NewOrganizationRepository(db)},
		NewReceivableWorkerResolver(db),
		receivablerepo.NewInvoiceRepository(db),
		nil,
	)
}

// crmReceivableReader menjadi adapter tipis CRM → receivable (status invoice, nomor/status kontrak) supaya
// evaluator Won dan respons SO tidak mengakses repository receivable langsung.
type crmReceivableReader struct {
	invoices  receivableservice.InvoiceService
	contracts receivableservice.ContractService
}

func (r crmReceivableReader) InvoiceStatus(ctx context.Context, scope coretenant.Scope, id string) (string, string, error) {
	inv, err := r.invoices.Get(ctx, scope, id)
	if err != nil {
		return "", "", err
	}
	return string(inv.Status), inv.InvoiceNumber, nil
}

func (r crmReceivableReader) ContractNumber(ctx context.Context, scope coretenant.Scope, id string) (string, error) {
	c, err := r.contracts.Get(ctx, scope, id)
	if err != nil {
		return "", err
	}
	return c.ContractNumber, nil
}

func (r crmReceivableReader) ContractInfo(ctx context.Context, scope coretenant.Scope, id string) (string, string, error) {
	c, err := r.contracts.Get(ctx, scope, id)
	if err != nil {
		return "", "", err
	}
	return string(c.Status), c.ContractNumber, nil
}
