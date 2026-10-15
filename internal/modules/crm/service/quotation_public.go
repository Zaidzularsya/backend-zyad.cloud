package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/publiclink"
)

// PublicLinkIdentity adalah identitas layanan untuk membangun scope tenant
// dari link publik (organization_id berasal dari baris link, bukan input).
const PublicLinkIdentity = "public-document-link"

// publicLinkGrace: link yang sudah kedaluwarsa lebih lama dari ini diperlakukan
// sama dengan token tidak dikenal.
const publicLinkGrace = 30 * 24 * time.Hour

const (
	maxResponderName = 150
	minResponderName = 2
	maxRevisionNote  = 2000
)

type PublicScopeResolver interface {
	ResolveWorkerOrganization(ctx context.Context, organizationID, serviceIdentity string) (coretenant.Context, error)
}

type LinkResolver interface {
	Resolve(ctx context.Context, token string) (publiclink.Link, error)
	Touch(ctx context.Context, link publiclink.Link)
}

// QuotationApprovedHook dipanggil setelah quotation approved (customer ATAU
// sales). Diisi rilis berikutnya (buat sales order draft); nil = tidak ada.
type QuotationApprovedHook interface {
	QuotationApproved(ctx context.Context, scope coretenant.Scope, q domain.Quotation, actorUserID string) error
}

func WithQuotationApprovedHook(h QuotationApprovedHook) QuotationServiceOption {
	return func(s *quotationService) { s.approvedHook = h }
}

type QuotationResponseNotifier interface {
	Notify(ctx context.Context, scope coretenant.Scope, q domain.Quotation, r domain.QuotationResponse) error
}

type PublicQuotationState string

const (
	PublicQuotationStateActive     PublicQuotationState = "active"
	PublicQuotationStateDecided    PublicQuotationState = "decided"
	PublicQuotationStateExpired    PublicQuotationState = "expired"
	PublicQuotationStateSuperseded PublicQuotationState = "superseded"
)

type PublicQuotationView struct {
	TenantName, QuotationNumber string
	Status                      domain.QuotationStatus
	State                       PublicQuotationState
	ValidUntil                  *time.Time
	Quotation                   domain.Quotation
	LastResponse                *domain.QuotationResponse
}

type PublicResponseInput struct {
	ResponderName, Note, IP, UserAgent string
	Categories                         []domain.RevisionCategory
}

var (
	ErrLinkInvalid             = publiclink.ErrLinkInvalid
	ErrQuotationNotRespondable = repository.ErrQuotationNotRespondable
	ErrInvalidResponse         = errors.New("responder_name 2-150 chars; revision needs >=1 known category; note required for other; note max 2000")
)

type PublicQuotationService struct {
	links      LinkResolver
	scopes     PublicScopeResolver
	quotations QuotationService
	responses  repository.QuotationResponseRepository
	issuers    repository.QuotationIssuerRepository
	activities repository.ActivityRepository
	notifier   QuotationResponseNotifier
	hook       QuotationApprovedHook
	now        func() time.Time
}

func NewPublicQuotationService(links LinkResolver, scopes PublicScopeResolver, quotations QuotationService,
	responses repository.QuotationResponseRepository, issuers repository.QuotationIssuerRepository,
	activities repository.ActivityRepository, notifier QuotationResponseNotifier, hook QuotationApprovedHook,
	now func() time.Time) *PublicQuotationService {
	if now == nil {
		now = time.Now
	}
	return &PublicQuotationService{links: links, scopes: scopes, quotations: quotations, responses: responses,
		issuers: issuers, activities: activities, notifier: notifier, hook: hook, now: now}
}

type publicDoc struct {
	link  publiclink.Link
	scope coretenant.Scope
	q     domain.Quotation
	state PublicQuotationState
}

// load memvalidasi token dan membangun scope tenant dari baris link. Semua
// penolakan (tidak dikenal, dicabut selain karena revisi, kedaluwarsa > 30 hari)
// menghasilkan ErrLinkInvalid yang sama agar tidak membocorkan keberadaan dokumen.
func (s *PublicQuotationService) load(ctx context.Context, token string) (publicDoc, error) {
	link, err := s.links.Resolve(ctx, token)
	if err != nil {
		return publicDoc{}, err
	}
	now := s.now()
	if link.DocumentType != publiclink.DocumentQuotation || now.After(link.ExpiresAt.Add(publicLinkGrace)) {
		return publicDoc{}, ErrLinkInvalid
	}
	tenantCtx, err := s.scopes.ResolveWorkerOrganization(ctx, link.OrganizationID, PublicLinkIdentity)
	if err != nil {
		var appErr *coreerrors.AppError
		if errors.As(err, &appErr) && (appErr.Code == "WORKER_ORGANIZATION_NOT_FOUND" || appErr.Code == "WORKER_ORGANIZATION_INACTIVE") {
			return publicDoc{}, ErrLinkInvalid
		}
		return publicDoc{}, err
	}
	scope, err := coretenant.NewScope(tenantCtx)
	if err != nil {
		return publicDoc{}, err
	}
	q, err := s.quotations.Get(ctx, scope, link.DocumentID)
	if err != nil {
		var appErr *coreerrors.AppError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &appErr) && appErr.Status == http.StatusNotFound) {
			return publicDoc{}, ErrLinkInvalid
		}
		return publicDoc{}, err
	}
	if q.Status == domain.QuotationStatusDraft {
		return publicDoc{}, ErrLinkInvalid
	}
	if link.RevokedAt != nil && q.Status != domain.QuotationStatusSuperseded {
		return publicDoc{}, ErrLinkInvalid
	}
	return publicDoc{link: link, scope: scope, q: q, state: s.stateOf(q, link, now)}, nil
}

func (s *PublicQuotationService) stateOf(q domain.Quotation, link publiclink.Link, now time.Time) PublicQuotationState {
	switch q.Status {
	case domain.QuotationStatusSuperseded:
		return PublicQuotationStateSuperseded
	case domain.QuotationStatusApproved, domain.QuotationStatusRejected, domain.QuotationStatusRevisionRequested:
		return PublicQuotationStateDecided
	case domain.QuotationStatusExpired:
		return PublicQuotationStateExpired
	}
	if now.After(link.ExpiresAt) {
		return PublicQuotationStateExpired
	}
	return PublicQuotationStateActive
}

func (s *PublicQuotationService) view(ctx context.Context, d publicDoc) (PublicQuotationView, error) {
	v := PublicQuotationView{
		QuotationNumber: d.q.QuotationNumber, Status: d.q.Status, State: d.state, ValidUntil: d.q.ValidUntil, Quotation: d.q,
	}
	if s.issuers != nil {
		if issuer, err := s.issuers.Find(ctx, d.scope); err == nil {
			v.TenantName = issuer.Name
		}
	}
	if d.state == PublicQuotationStateDecided && s.responses != nil {
		if list, err := s.responses.ListByQuotation(ctx, d.scope, d.q.ID); err == nil && len(list) > 0 {
			v.LastResponse = &list[0]
		}
	}
	return v, nil
}

func (s *PublicQuotationService) View(ctx context.Context, token string) (PublicQuotationView, error) {
	d, err := s.load(ctx, token)
	if err != nil {
		return PublicQuotationView{}, err
	}
	s.links.Touch(ctx, d.link)
	return s.view(ctx, d)
}

func (s *PublicQuotationService) PDF(ctx context.Context, token string) (QuotationPDF, error) {
	d, err := s.load(ctx, token)
	if err != nil {
		return QuotationPDF{}, err
	}
	return s.quotations.PDF(ctx, d.scope, d.q.ID)
}

func (s *PublicQuotationService) Approve(ctx context.Context, token string, in PublicResponseInput) (PublicQuotationView, error) {
	in.ResponderName = strings.TrimSpace(in.ResponderName)
	if !validResponderName(in.ResponderName) {
		return PublicQuotationView{}, ErrInvalidResponse
	}
	in.Categories, in.Note = nil, strings.TrimSpace(in.Note)
	return s.respond(ctx, token, domain.QuotationResponseApproved, in)
}

func (s *PublicQuotationService) RequestRevision(ctx context.Context, token string, in PublicResponseInput) (PublicQuotationView, error) {
	in.ResponderName, in.Note = strings.TrimSpace(in.ResponderName), strings.TrimSpace(in.Note)
	if !validResponderName(in.ResponderName) || !validRevision(in.Categories, in.Note) {
		return PublicQuotationView{}, ErrInvalidResponse
	}
	return s.respond(ctx, token, domain.QuotationResponseRevisionRequested, in)
}

func validResponderName(name string) bool {
	n := utf8.RuneCountInString(name)
	return n >= minResponderName && n <= maxResponderName
}

func validRevision(categories []domain.RevisionCategory, note string) bool {
	if len(categories) == 0 || utf8.RuneCountInString(note) > maxRevisionNote {
		return false
	}
	seen := map[domain.RevisionCategory]bool{}
	for _, c := range categories {
		if !c.IsValid() || seen[c] {
			return false
		}
		seen[c] = true
	}
	return !seen[domain.RevisionOther] || note != ""
}

func (s *PublicQuotationService) respond(ctx context.Context, token string, action domain.QuotationResponseAction, in PublicResponseInput) (PublicQuotationView, error) {
	d, err := s.load(ctx, token)
	if err != nil {
		return PublicQuotationView{}, err
	}
	if d.state != PublicQuotationStateActive {
		return PublicQuotationView{}, ErrQuotationNotRespondable
	}
	q, resp, err := s.responses.Respond(ctx, d.scope, repository.RecordResponseParams{
		QuotationID: d.q.ID, LinkID: d.link.ID, Action: action, Categories: in.Categories, Note: in.Note,
		ResponderName: in.ResponderName, IP: in.IP, UserAgent: truncateRunes(in.UserAgent, 500),
	})
	if err != nil {
		return PublicQuotationView{}, err
	}

	// Setelah respons tercatat, kegagalan efek samping tidak boleh membatalkannya.
	s.logActivity(ctx, d.scope, q, resp)
	if s.notifier != nil {
		if err := s.notifier.Notify(ctx, d.scope, q, resp); err != nil {
			slog.WarnContext(ctx, "quotation response notification failed", "quotation_id", q.ID, "error", err)
		}
	}
	if action == domain.QuotationResponseApproved && s.hook != nil {
		if err := s.hook.QuotationApproved(ctx, d.scope, q, ""); err != nil {
			slog.WarnContext(ctx, "quotation approved hook failed", "quotation_id", q.ID, "error", err)
		}
	}

	d.q, d.state = q, PublicQuotationStateDecided
	v, _ := s.view(ctx, d)
	v.LastResponse = &resp
	return v, nil
}

func (s *PublicQuotationService) logActivity(ctx context.Context, scope coretenant.Scope, q domain.Quotation, r domain.QuotationResponse) {
	if s.activities == nil || q.DealID == nil {
		return
	}
	meta := map[string]any{
		"action": string(r.Action), "responder_name": r.ResponderName,
		"quotation_number": q.QuotationNumber, "quotation_id": q.ID,
	}
	subject := "Customer menyetujui penawaran " + q.QuotationNumber
	description := ""
	if r.Action == domain.QuotationResponseRevisionRequested {
		subject = "Customer meminta revisi penawaran " + q.QuotationNumber
		codes := make([]string, 0, len(r.Categories))
		for _, c := range r.Categories {
			codes = append(codes, string(c))
		}
		meta["categories"] = codes
		meta["note"] = r.Note
		description = r.Note
	}
	if _, err := s.activities.Create(ctx, scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityDeal, RelatedEntityID: *q.DealID, Type: domain.ActivityTypeQuotationResponse,
		Subject: subject, Description: description, Status: domain.ActivityStatusCompleted, Metadata: meta,
	}); err != nil {
		slog.WarnContext(ctx, "quotation response activity failed", "quotation_id", q.ID, "error", err)
	}
}

func truncateRunes(v string, max int) string {
	if utf8.RuneCountInString(v) <= max {
		return v
	}
	return string([]rune(v)[:max])
}
