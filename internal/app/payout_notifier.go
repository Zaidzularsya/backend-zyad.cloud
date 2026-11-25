package app

import (
	"context"
	"log/slog"
	"strings"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	"zyad.cloud/internal/modules/organization/model"
	receivablerepo "zyad.cloud/internal/modules/receivable/repository"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

type payoutOrganizations interface {
	FindByID(ctx context.Context, id string) (model.Organization, error)
}

// payoutNotifier mengirim email siklus rekening pencairan ke owner organisasi (satu event per owner).
// Nomor rekening selalu dimask. Kegagalan hanya dicatat: aksi yang memicunya sudah tersimpan.
type payoutNotifier struct {
	owners      workspaceOwners
	orgs        payoutOrganizations
	publisher   notificationPublisher
	appName     string
	frontendURL string
	locale      string
}

var _ receivableservice.PayoutAccountNotifier = payoutNotifier{}

func (n payoutNotifier) NotifyPayoutAccount(ctx context.Context, event string, acc receivablerepo.PayoutAccount) {
	owners, err := n.owners.ListOwners(ctx, acc.OrganizationID)
	if err != nil {
		slog.WarnContext(ctx, "receivable: list owners for payout notification failed", "event", event, "organization_id", acc.OrganizationID, "error", err)
		return
	}
	if len(owners) == 0 {
		slog.WarnContext(ctx, "receivable: payout notification has no owner with email", "event", event, "organization_id", acc.OrganizationID)
		return
	}
	// Template menandai organization_name wajib; pakai "-" bila lookup gagal atau nama kosong.
	orgName := "-"
	if org, err := n.orgs.FindByID(ctx, acc.OrganizationID); err != nil {
		slog.WarnContext(ctx, "receivable: read organization for payout notification failed", "organization_id", acc.OrganizationID, "error", err)
	} else if name := strings.TrimSpace(org.Name); name != "" {
		orgName = name
	}
	note := strings.TrimSpace(acc.ReviewNote)
	if note == "" {
		note = "-"
	}
	for _, m := range owners {
		_, err := n.publisher.Publish(ctx, notificationpublisher.Event{
			Type: event, OrganizationID: acc.OrganizationID, UserID: m.UserID,
			Recipient: notificationdomain.NotificationRecipient{Type: "user", UserID: m.UserID, Name: m.Name, Email: m.Email},
			Payload: map[string]any{
				"app_name": n.appName, "user_name": m.Name, "organization_name": orgName,
				"bank_name": acc.BankName, "account_number_masked": receivableservice.MaskAccountNumber(acc.AccountNumber),
				"account_holder_name": acc.AccountHolderName, "review_note": singleLine(note),
				"settlement_url": strings.TrimRight(n.frontendURL, "/") + "/app/account/bank",
			},
			Locale: n.locale,
		})
		if err != nil {
			slog.WarnContext(ctx, "receivable: publish payout account notification failed", "event", event, "payout_account_id", acc.ID, "error", err)
		}
	}
}
