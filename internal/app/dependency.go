package app

import (
	"log/slog"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/middleware"
	notificationhandler "zyad.cloud/internal/core/notification/handler"
	permissionhandler "zyad.cloud/internal/core/permission/handler"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	assethandler "zyad.cloud/internal/modules/asset/handler"
	cataloghandler "zyad.cloud/internal/modules/catalog/handler"
	crmhandler "zyad.cloud/internal/modules/crm/handler"
	financehandler "zyad.cloud/internal/modules/finance/handler"
	landinghandler "zyad.cloud/internal/modules/landing/handler"
	mailboxhandler "zyad.cloud/internal/modules/mailbox/handler"
	organizationhandler "zyad.cloud/internal/modules/organization/handler"
	producthandler "zyad.cloud/internal/modules/product/handler"
	receivablehandler "zyad.cloud/internal/modules/receivable/handler"
	userhandler "zyad.cloud/internal/modules/user/handler"
	whatsapphandler "zyad.cloud/internal/modules/whatsapp/handler"
	pgdatabase "zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/redis"
	"zyad.cloud/internal/platform/storage"
)

type Dependencies struct {
	Config                           config.Config
	Logger                           *slog.Logger
	DB                               *pgdatabase.Pool
	Redis                            *redis.Client
	NotificationHandler              *notificationhandler.NotificationHandler
	NotificationLogHandler           *notificationhandler.LogHandler
	NotificationPreferenceHandler    *notificationhandler.PreferenceHandler
	NotificationTemplateHandler      *notificationhandler.TemplateHandler
	NotificationVariableHandler      *notificationhandler.VariableHandler
	PermissionHandler                *permissionhandler.Handler
	PlatformProductHandler           *producthandler.PlatformProductHandler
	DokuWebhookHandler               *receivablehandler.DokuWebhookHandler
	OrganizationDomainHandler        *organizationhandler.DomainHandler
	OrganizationEntitlementHandler   *organizationhandler.EntitlementHandler
	OrganizationImpersonationHandler *organizationhandler.ImpersonationHandler
	OrganizationOnboardingHandler    *organizationhandler.OnboardingHandler
	OrganizationPlatformHandler      *organizationhandler.PlatformHandler
	OrganizationSelfHandler          *organizationhandler.SelfHandler
	OrganizationSwitchHandler        *organizationhandler.SwitchHandler
	UserAuthHandler                  *userhandler.AuthHandler
	UserHandler                      *userhandler.UserHandler
	LandingAdminPageHandler          *landinghandler.AdminPageHandler
	LandingAdminSectionHandler       *landinghandler.AdminSectionHandler
	LandingAdminDocumentHandler      *landinghandler.AdminDocumentHandler
	LandingAdminBrandingHandler      *landinghandler.AdminBrandingHandler
	LandingAdminDomainHandler        *landinghandler.AdminDomainHandler
	LandingAdminFormHandler          *landinghandler.AdminFormHandler
	LandingAdminSubmissionHandler    *landinghandler.AdminSubmissionHandler
	PublicLandingHandler             *landinghandler.PublicLandingHandler
	LandingAdminCTAHandler           *landinghandler.AdminCTAHandler
	LandingAdminTemplateHandler      *landinghandler.AdminTemplateHandler
	LandingAdminMediaHandler         *landinghandler.AdminMediaHandler
	LandingAdminNavigationHandler    *landinghandler.AdminNavigationHandler
	LandingAdminPricingHandler       *landinghandler.AdminPricingHandler
	LandingAdminRevisionHandler      *landinghandler.AdminRevisionHandler
	LandingAdminScheduleHandler      *landinghandler.AdminScheduleHandler
	LandingAdminIntegrationHandler   *landinghandler.AdminIntegrationHandler
	CRMEntitlementChecker            middleware.EntitlementChecker
	CRMCompanyHandler                *crmhandler.CompanyHandler
	CRMContactHandler                *crmhandler.ContactHandler
	CRMLeadHandler                   *crmhandler.LeadHandler
	CRMLeadAttachmentHandler         *crmhandler.LeadAttachmentHandler
	CRMContactAttachmentHandler      *crmhandler.ContactAttachmentHandler
	MailboxHandler                   *mailboxhandler.Handler
	CatalogProductHandler            *cataloghandler.ProductHandler
	CatalogCategoryHandler           *cataloghandler.CategoryHandler
	CatalogFeatureHandler            *cataloghandler.FeatureHandler
	CatalogPublicListingHandler      *cataloghandler.PublicListingHandler
	CRMLeadDashboardHandler          *crmhandler.LeadDashboardHandler
	CRMSettingsHandler               *crmhandler.CRMSettingsHandler
	CRMMemberHandler                 *crmhandler.MemberHandler
	CRMPipelineHandler               *crmhandler.PipelineHandler
	CRMDealHandler                   *crmhandler.DealHandler
	CRMActivityHandler               *crmhandler.ActivityHandler
	CRMQuotationHandler              *crmhandler.QuotationHandler
	CRMPublicQuotationHandler        *crmhandler.PublicQuotationHandler
	CRMSelfServeHandler              *crmhandler.SelfServeHandler
	CRMSalesOrderHandler             *crmhandler.SalesOrderHandler
	CRMDealOrdersHandler             *crmhandler.DealOrdersHandler
	ReceivableContractHandler        *receivablehandler.ContractHandler
	ReceivableOverviewHandler        *receivablehandler.OverviewHandler
	ReceivableAccountHandler         *receivablehandler.AccountHandler
	ReceivableInvoiceHandler         *receivablehandler.InvoiceHandler
	ReceivablePaymentHandler         *receivablehandler.PaymentHandler
	ReceivableSettingsHandler        *receivablehandler.SettingsHandler
	ReceivablePublicInvoiceHandler   *receivablehandler.PublicInvoiceHandler
	CRMIntegrationHandler            *crmhandler.IntegrationHandler
	WhatsAppEntitlementChecker       middleware.EntitlementChecker
	WhatsAppSessionHandler           *whatsapphandler.SessionHandler
	WhatsAppWebhookHandler           *whatsapphandler.WebhookHandler
	WhatsAppConversationHandler      *whatsapphandler.ConversationHandler
	AdminAssetHandler                *assethandler.AdminAssetHandler
	PlatformAssetHandler             *assethandler.PlatformAssetHandler
	PlatformFinanceCoAHandler        *financehandler.PlatformCoAHandler
	PlatformFinanceJournalHandler    *financehandler.PlatformJournalHandler
	PlatformFinanceReportHandler     *financehandler.PlatformReportHandler
	PlatformFinanceCashBankHandler   *financehandler.PlatformCashBankHandler
	PlatformFinanceARAPHandler       *financehandler.PlatformARAPHandler
	PlatformFinanceFixedAssetHandler *financehandler.PlatformFixedAssetHandler
	PlatformFinanceTaxHandler        *financehandler.PlatformTaxHandler
	MediaStorage                     storage.MediaStorage
	PermissionChecker                permissionmiddleware.CombinedPermissionChecker
	Authenticator                    middleware.AccessTokenAuthenticator
	OrganizationResolver             middleware.AuthenticatedOrganizationResolver
	PublicHostResolver               middleware.PublicHostResolver
}
