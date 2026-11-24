package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"zyad.cloud/internal/modules/receivable/repository"
)

var (
	ErrInvalidPayoutAccount = errors.New("invalid payout account")
	ErrPayoutNoteRequired   = errors.New("payout account review note is required")
)

const (
	EventPayoutSubmitted = "receivable.payout_account_submitted"
	EventPayoutVerified  = "receivable.payout_account_verified"
	EventPayoutRejected  = "receivable.payout_account_rejected"
	EventPayoutAssigned  = "receivable.payout_account_assigned"

	payoutNumberMin = 5
	payoutNumberMax = 34
	payoutHolderMin = 3
	payoutHolderMax = 100
	payoutNoteMax   = 500
)

type PayoutAccountInput struct{ BankCode, AccountNumber, AccountHolderName string }

type PayoutOverview struct {
	Verified, Pending *repository.PayoutAccount
	History           []repository.PayoutAccount
}

// PayoutAccountNotifier dipanggil setelah mutasi sukses; kegagalan notifikasi tidak menggagalkan aksi.
type PayoutAccountNotifier interface {
	NotifyPayoutAccount(ctx context.Context, event string, acc repository.PayoutAccount)
}

type PayoutAccountService interface {
	Overview(ctx context.Context, orgID string) (PayoutOverview, error)
	Submit(ctx context.Context, orgID string, in PayoutAccountInput, a repository.PayoutAudit) (repository.PayoutAccount, error)
	Cancel(ctx context.Context, orgID, id string, a repository.PayoutAudit) (repository.PayoutAccount, error)
	ListPending(ctx context.Context, limit, offset int) ([]repository.PayoutAccountWithOrg, int64, error)
	History(ctx context.Context, orgID string) ([]repository.PayoutAccount, error)
	Verify(ctx context.Context, id, note string, a repository.PayoutAudit) (repository.PayoutAccount, error)
	Reject(ctx context.Context, id, note string, a repository.PayoutAudit) (repository.PayoutAccount, error)
	Assign(ctx context.Context, orgID string, in PayoutAccountInput, note string, a repository.PayoutAudit) (repository.PayoutAccount, error)
	Summary(ctx context.Context, orgIDs []string) (map[string]repository.PayoutSummary, error)
}

type payoutAccountService struct {
	repo     repository.PayoutAccountRepository
	ledger   repository.SettlementLedgerRepository
	notifier PayoutAccountNotifier
	now      func() time.Time
}

func NewPayoutAccountService(repo repository.PayoutAccountRepository, ledger repository.SettlementLedgerRepository, notifier PayoutAccountNotifier) PayoutAccountService {
	return &payoutAccountService{repo: repo, ledger: ledger, notifier: notifier, now: time.Now}
}

// MaskAccountNumber: "••••" + 4 digit terakhir (aturan tunggal ada di repository).
func MaskAccountNumber(n string) string { return repository.MaskAccountNumber(n) }

func invalidPayout(detail string) error {
	return &validationError{kind: ErrInvalidPayoutAccount, detail: detail}
}

// normalizePayoutInput membuang spasi/tanda hubung dari nomor rekening lalu memvalidasi semua kolom.
func normalizePayoutInput(in PayoutAccountInput) (PayoutAccountInput, repository.NewPayoutAccount, error) {
	var out repository.NewPayoutAccount
	bank, ok := FindBank(strings.TrimSpace(in.BankCode))
	if !ok {
		return in, out, invalidPayout("Bank tidak dikenali.")
	}
	number := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(in.AccountNumber))
	if n := len(number); n < payoutNumberMin || n > payoutNumberMax || strings.Trim(number, "0123456789") != "" {
		return in, out, invalidPayout("Nomor rekening harus 5 sampai 34 digit angka.")
	}
	holder := strings.Join(strings.Fields(in.AccountHolderName), " ")
	if n := utf8.RuneCountInString(holder); n < payoutHolderMin || n > payoutHolderMax {
		return in, out, invalidPayout("Nama pemilik rekening harus 3 sampai 100 karakter.")
	}
	out = repository.NewPayoutAccount{BankCode: bank.Code, BankName: bank.Name, AccountNumber: number, AccountHolderName: holder}
	return in, out, nil
}

func (s *payoutAccountService) notify(ctx context.Context, event string, acc repository.PayoutAccount) {
	if s.notifier != nil {
		s.notifier.NotifyPayoutAccount(ctx, event, acc)
	}
}

func (s *payoutAccountService) Overview(ctx context.Context, orgID string) (PayoutOverview, error) {
	hist, err := s.repo.ListByOrganization(ctx, orgID)
	if err != nil {
		return PayoutOverview{}, err
	}
	ov := PayoutOverview{History: hist}
	for i := range hist {
		switch hist[i].Status {
		case "verified":
			if ov.Verified == nil {
				ov.Verified = &hist[i]
			}
		case "pending":
			if ov.Pending == nil {
				ov.Pending = &hist[i]
			}
		}
	}
	return ov, nil
}

func (s *payoutAccountService) Submit(ctx context.Context, orgID string, in PayoutAccountInput, a repository.PayoutAudit) (repository.PayoutAccount, error) {
	orgID = strings.TrimSpace(orgID)
	_, n, err := normalizePayoutInput(in)
	if err != nil {
		return repository.PayoutAccount{}, err
	}
	if orgID == "" {
		return repository.PayoutAccount{}, ErrSettlementBeneficiaryNotFound
	}
	if err := requireCustomerOrg(ctx, s.ledger, orgID); err != nil {
		return repository.PayoutAccount{}, err
	}
	n.OrganizationID, n.ActorUserID, n.At = orgID, a.ActorUserID, s.now().UTC()
	acc, err := s.repo.InsertPending(ctx, n, a)
	if err != nil {
		return repository.PayoutAccount{}, err
	}
	s.notify(ctx, EventPayoutSubmitted, acc)
	return acc, nil
}

func (s *payoutAccountService) Cancel(ctx context.Context, orgID, id string, a repository.PayoutAudit) (repository.PayoutAccount, error) {
	if strings.TrimSpace(orgID) == "" {
		return repository.PayoutAccount{}, repository.ErrPayoutAccountNotFound
	}
	return s.repo.Cancel(ctx, orgID, id, s.now().UTC(), a)
}

func (s *payoutAccountService) ListPending(ctx context.Context, limit, offset int) ([]repository.PayoutAccountWithOrg, int64, error) {
	return s.repo.ListPending(ctx, limit, offset)
}

func (s *payoutAccountService) History(ctx context.Context, orgID string) ([]repository.PayoutAccount, error) {
	return s.repo.ListByOrganization(ctx, orgID)
}

func cleanNote(note string, required bool) (string, error) {
	note = strings.TrimSpace(note)
	if required && note == "" {
		return "", ErrPayoutNoteRequired
	}
	if utf8.RuneCountInString(note) > payoutNoteMax {
		return "", invalidPayout("Catatan terlalu panjang.")
	}
	return note, nil
}

func (s *payoutAccountService) Verify(ctx context.Context, id, note string, a repository.PayoutAudit) (repository.PayoutAccount, error) {
	note, err := cleanNote(note, false)
	if err != nil {
		return repository.PayoutAccount{}, err
	}
	acc, err := s.repo.Verify(ctx, id, note, s.now().UTC(), a)
	if err != nil {
		return repository.PayoutAccount{}, err
	}
	s.notify(ctx, EventPayoutVerified, acc)
	return acc, nil
}

func (s *payoutAccountService) Reject(ctx context.Context, id, note string, a repository.PayoutAudit) (repository.PayoutAccount, error) {
	note, err := cleanNote(note, true)
	if err != nil {
		return repository.PayoutAccount{}, err
	}
	acc, err := s.repo.Reject(ctx, id, note, s.now().UTC(), a)
	if err != nil {
		return repository.PayoutAccount{}, err
	}
	s.notify(ctx, EventPayoutRejected, acc)
	return acc, nil
}

func (s *payoutAccountService) Assign(ctx context.Context, orgID string, in PayoutAccountInput, note string, a repository.PayoutAudit) (repository.PayoutAccount, error) {
	orgID = strings.TrimSpace(orgID)
	note, err := cleanNote(note, true)
	if err != nil {
		return repository.PayoutAccount{}, err
	}
	_, n, err := normalizePayoutInput(in)
	if err != nil {
		return repository.PayoutAccount{}, err
	}
	if orgID == "" {
		return repository.PayoutAccount{}, ErrSettlementBeneficiaryNotFound
	}
	if err := requireCustomerOrg(ctx, s.ledger, orgID); err != nil {
		return repository.PayoutAccount{}, err
	}
	n.OrganizationID, n.ActorUserID, n.At = orgID, a.ActorUserID, s.now().UTC()
	acc, err := s.repo.AssignVerified(ctx, n, note, a)
	if err != nil {
		return repository.PayoutAccount{}, err
	}
	s.notify(ctx, EventPayoutAssigned, acc)
	return acc, nil
}

func (s *payoutAccountService) Summary(ctx context.Context, orgIDs []string) (map[string]repository.PayoutSummary, error) {
	return s.repo.SummaryForOrganizations(ctx, orgIDs)
}
