package service

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

const domainVerificationPrefix = "_zyad-verification."

// domainVerifyCooldown membatasi frekuensi lookup DNS per domain.
const domainVerifyCooldown = 30 * time.Second

var domainLabelPattern = regexp.MustCompile(
	`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`,
)

type DomainStore interface {
	Create(context.Context, repository.CreateDomainParams) (model.OrganizationDomain, error)
	CreateCustomDomain(
		context.Context,
		repository.CreateDomainParams,
		func(usedSameType int64) error,
	) (model.OrganizationDomain, error)
	FindByID(context.Context, string, string) (model.OrganizationDomain, error)
	ListByOrganization(
		context.Context,
		string,
		repository.DomainListFilter,
	) ([]model.OrganizationDomain, int64, error)
	UpdateVerification(
		context.Context,
		string,
		string,
		repository.UpdateDomainVerificationParams,
	) (model.OrganizationDomain, error)
	UpdateChallenge(
		context.Context,
		string,
		string,
		string,
		time.Time,
	) (model.OrganizationDomain, error)
	Activate(
		context.Context,
		string,
		string,
		bool,
		time.Time,
		string,
	) (model.OrganizationDomain, error)
	SetPrimary(context.Context, string, string, time.Time, string) (model.OrganizationDomain, error)
	Delete(context.Context, string, string, time.Time, string) error
}

type DomainService struct {
	store                 DomainStore
	verifier              DomainVerifier
	guard                 DomainBillingGuard
	platformPrimaryDomain string
	now                   func() time.Time
	generateToken         func(int) (string, error)
}

type DomainBillingGuard interface {
	RequireFeature(
		context.Context,
		string,
		string,
	) (model.Entitlement, error)
	RequireQuotaValue(
		context.Context,
		string,
		string,
		string,
		int64,
		int64,
	) error
}

type DomainServiceOption func(*DomainService)

func WithDomainBillingGuard(guard DomainBillingGuard) DomainServiceOption {
	return func(service *DomainService) {
		service.guard = guard
	}
}

func NewDomainService(
	store DomainStore,
	verifier DomainVerifier,
	platformPrimaryDomain string,
	options ...DomainServiceOption,
) *DomainService {
	service := &DomainService{
		store:                 store,
		verifier:              verifier,
		platformPrimaryDomain: normalizeDomainHost(platformPrimaryDomain),
		now:                   time.Now,
		generateToken:         coreauth.NewRandomToken,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *DomainService) List(
	ctx context.Context,
	organizationID string,
	query dto.DomainListQuery,
) ([]dto.DomainResponse, dto.PaginationMeta, error) {
	if !validUUID(organizationID) {
		return nil, dto.PaginationMeta{}, validationError(
			"organization_id must be a valid UUID",
		)
	}
	filter, page, perPage, err := domainListFilter(query)
	if err != nil {
		return nil, dto.PaginationMeta{}, err
	}
	domains, total, err := s.store.ListByOrganization(
		ctx,
		strings.TrimSpace(organizationID),
		filter,
	)
	if err != nil {
		return nil, dto.PaginationMeta{}, mapDomainError(
			"DOMAIN_LIST_FAILED",
			"failed to list organization domains",
			err,
		)
	}
	items := make([]dto.DomainResponse, 0, len(domains))
	for _, domain := range domains {
		items = append(items, domainResponse(domain))
	}
	return items, dto.PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: int(math.Ceil(float64(total) / float64(perPage))),
	}, nil
}

func (s *DomainService) Create(
	ctx context.Context,
	organizationID string,
	request dto.CreateDomainRequest,
	actorUserID string,
) (dto.DomainChallengeResponse, error) {
	if !validUUID(organizationID) {
		return dto.DomainChallengeResponse{}, validationError(
			"organization_id must be a valid UUID",
		)
	}
	if request.IsPrimary {
		return dto.DomainChallengeResponse{}, validationError(
			"is_primary can only be set after domain verification",
		)
	}
	domainType := model.DomainType(strings.TrimSpace(request.Type))
	if domainType != model.DomainTypeSubdomain &&
		domainType != model.DomainTypeCustom {
		return dto.DomainChallengeResponse{}, validationError(
			"domain type must be subdomain or custom",
		)
	}
	host := normalizeDomainHost(request.CanonicalHost)
	if !validDomainHost(host) {
		return dto.DomainChallengeResponse{}, validationError(
			"canonical_host must be a valid DNS hostname",
		)
	}
	if len(verificationRecordName(host)) > 253 {
		return dto.DomainChallengeResponse{}, validationError(
			"canonical_host is too long for the verification record",
		)
	}
	if domainType == model.DomainTypeSubdomain {
		if s.platformPrimaryDomain == "" {
			return dto.DomainChallengeResponse{}, coreerrors.New(
				"PLATFORM_PRIMARY_DOMAIN_REQUIRED",
				"platform primary domain is required for organization subdomains",
				http.StatusServiceUnavailable,
			)
		}
		suffix := "." + s.platformPrimaryDomain
		if !strings.HasSuffix(host, suffix) ||
			strings.Contains(strings.TrimSuffix(host, suffix), ".") {
			return dto.DomainChallengeResponse{}, validationError(
				"subdomain must be a direct child of the platform primary domain",
			)
		}
	}
	if domainType == model.DomainTypeCustom {
		if err := s.requireCustomDomainFeature(ctx, strings.TrimSpace(organizationID)); err != nil {
			return dto.DomainChallengeResponse{}, err
		}
	}
	if domainType == model.DomainTypeSubdomain {
		// Subdomain platform berada di zona DNS milik platform sehingga
		// tenant tidak mungkin memasang TXT challenge — langsung aktif.
		// Sertifikat TLS-nya juga tanggung jawab platform (wildcard cert di
		// reverse proxy), bukan sesuatu yang perlu diprovision per-domain
		// oleh aplikasi, jadi ssl_status tidak pernah "pending" selamanya.
		domain, err := s.store.Create(ctx, repository.CreateDomainParams{
			OrganizationID: strings.TrimSpace(organizationID),
			Type:           domainType,
			CanonicalHost:  host,
			SSLStatus:      model.DomainSSLStatusNotRequired,
			ActorUserID:    strings.TrimSpace(actorUserID),
		})
		if err != nil {
			return dto.DomainChallengeResponse{}, mapDomainError(
				"DOMAIN_CREATE_FAILED",
				"failed to create organization domain",
				err,
			)
		}
		activated, err := s.markVerifiedAndActivate(
			ctx,
			strings.TrimSpace(organizationID),
			domain.ID,
			strings.TrimSpace(actorUserID),
			s.now().UTC(),
		)
		if err != nil {
			return dto.DomainChallengeResponse{}, err
		}
		return dto.DomainChallengeResponse{
			Domain:        domainResponse(activated),
			ChallengeType: "auto_verified",
		}, nil
	}
	token, err := s.generateToken(32)
	if err != nil {
		return dto.DomainChallengeResponse{}, coreerrors.Wrap(
			"DOMAIN_CHALLENGE_GENERATION_FAILED",
			"failed to generate domain verification challenge",
			http.StatusInternalServerError,
			err,
		)
	}
	organizationIDTrimmed := strings.TrimSpace(organizationID)
	// Quota (semua custom domain yang belum dihapus, termasuk pending/failed,
	// supaya namespace host global tidak bisa dipenuhi lewat domain yang
	// tidak pernah diverifikasi) dihitung ulang dan dicek di dalam transaksi
	// yang sama dengan insert-nya, dengan organization row dikunci — supaya
	// dua request Create bersamaan tidak bisa sama-sama lolos cek quota lalu
	// sama-sama insert (TOCTOU race).
	domain, err := s.store.CreateCustomDomain(
		ctx,
		repository.CreateDomainParams{
			OrganizationID:            organizationIDTrimmed,
			Type:                      domainType,
			CanonicalHost:             host,
			VerificationChallengeHash: coreauth.HashToken(token),
			SSLStatus:                 model.DomainSSLStatusPending,
			ActorUserID:               strings.TrimSpace(actorUserID),
		},
		func(used int64) error {
			if s.guard == nil {
				return nil
			}
			return s.guard.RequireQuotaValue(
				ctx,
				organizationIDTrimmed,
				featureDomainMaxCustomDomains,
				"limit",
				used,
				1,
			)
		},
	)
	if err != nil {
		var quotaErr *repository.QuotaCheckError
		if errors.As(err, &quotaErr) {
			return dto.DomainChallengeResponse{}, quotaErr.Err
		}
		return dto.DomainChallengeResponse{}, mapDomainError(
			"DOMAIN_CREATE_FAILED",
			"failed to create organization domain",
			err,
		)
	}
	return dto.DomainChallengeResponse{
		Domain:        domainResponse(domain),
		ChallengeType: "dns_txt",
		RecordName:    verificationRecordName(domain.CanonicalHost),
		RecordValue:   token,
	}, nil
}

const (
	featureDomainEnabled          = "domain.enabled"
	featureDomainMaxCustomDomains = "domain.max_custom_domains"
)

// requireCustomDomainFeature hanya mengecek entitlement domain.enabled.
// Pengecekan quota (domain.max_custom_domains) dilakukan belakangan, di
// dalam transaksi terkunci CreateCustomDomain, karena nilai "used"-nya harus
// segar pada saat insert, bukan pada saat validasi awal ini.
func (s *DomainService) requireCustomDomainFeature(
	ctx context.Context,
	organizationID string,
) error {
	if s == nil || s.guard == nil {
		return nil
	}
	_, err := s.guard.RequireFeature(ctx, organizationID, featureDomainEnabled)
	return err
}

func (s *DomainService) Verify(
	ctx context.Context,
	organizationID string,
	domainID string,
	actorUserID string,
) (dto.DomainResponse, error) {
	if !validUUID(organizationID) || !validUUID(domainID) {
		return dto.DomainResponse{}, validationError(
			"organization_id and domain_id must be valid UUIDs",
		)
	}
	domain, err := s.store.FindByID(ctx, organizationID, domainID)
	if err != nil {
		return dto.DomainResponse{}, mapDomainError(
			"DOMAIN_QUERY_FAILED",
			"failed to get organization domain",
			err,
		)
	}
	if domain.Status == model.DomainStatusActive {
		// Sudah aktif — jangan re-run DNS check yang bisa menjatuhkan
		// domain produksi ke failed; verify bersifat idempoten.
		return domainResponse(domain), nil
	}
	if domain.Status == model.DomainStatusDisabled {
		return dto.DomainResponse{}, coreerrors.New(
			"DOMAIN_DISABLED",
			"organization domain is disabled",
			http.StatusConflict,
		)
	}
	now := s.now().UTC()
	if domain.LastVerificationAt != nil &&
		now.Sub(*domain.LastVerificationAt) < domainVerifyCooldown {
		return dto.DomainResponse{}, coreerrors.New(
			"DOMAIN_VERIFICATION_RATE_LIMITED",
			"domain verification was attempted too recently, retry shortly",
			http.StatusTooManyRequests,
		)
	}
	if domain.Type == model.DomainTypeSubdomain {
		// Subdomain platform tidak memakai DNS challenge (lihat Create).
		activated, err := s.markVerifiedAndActivate(
			ctx,
			organizationID,
			domain.ID,
			strings.TrimSpace(actorUserID),
			now,
		)
		if err != nil {
			return dto.DomainResponse{}, err
		}
		return domainResponse(activated), nil
	}
	if s.verifier == nil {
		return dto.DomainResponse{}, coreerrors.New(
			"DOMAIN_VERIFIER_REQUIRED",
			"domain verifier is not configured",
			http.StatusServiceUnavailable,
		)
	}
	if domain.VerificationChallengeHash == "" {
		return dto.DomainResponse{}, coreerrors.New(
			"DOMAIN_CHALLENGE_MISSING",
			"domain has no verification challenge, regenerate it first",
			http.StatusConflict,
		)
	}
	err = s.verifier.VerifyTXT(
		ctx,
		verificationRecordName(domain.CanonicalHost),
		domain.VerificationChallengeHash,
	)
	if errors.Is(err, ErrDomainChallengeNotFound) {
		failed, updateErr := s.store.UpdateVerification(
			ctx,
			organizationID,
			domainID,
			repository.UpdateDomainVerificationParams{
				Status:            model.DomainStatusFailed,
				VerificationError: "DNS TXT verification challenge was not found",
				AttemptedAt:       now,
				ActorUserID:       strings.TrimSpace(actorUserID),
			},
		)
		if updateErr != nil {
			return dto.DomainResponse{}, mapDomainError(
				"DOMAIN_VERIFICATION_UPDATE_FAILED",
				"failed to record domain verification result",
				updateErr,
			)
		}
		return domainResponse(failed), coreerrors.New(
			"DOMAIN_VERIFICATION_FAILED",
			"DNS TXT verification challenge was not found",
			http.StatusConflict,
		)
	}
	if err != nil {
		return dto.DomainResponse{}, coreerrors.Wrap(
			"DOMAIN_VERIFICATION_UNAVAILABLE",
			"domain verification resolver is unavailable",
			http.StatusServiceUnavailable,
			err,
		)
	}
	activated, err := s.markVerifiedAndActivate(
		ctx,
		organizationID,
		domainID,
		strings.TrimSpace(actorUserID),
		now,
	)
	if err != nil {
		return dto.DomainResponse{}, err
	}
	return domainResponse(activated), nil
}

func (s *DomainService) markVerifiedAndActivate(
	ctx context.Context,
	organizationID string,
	domainID string,
	actorUserID string,
	now time.Time,
) (model.OrganizationDomain, error) {
	verified, err := s.store.UpdateVerification(
		ctx,
		organizationID,
		domainID,
		repository.UpdateDomainVerificationParams{
			Status:      model.DomainStatusVerified,
			VerifiedAt:  &now,
			AttemptedAt: now,
			ActorUserID: actorUserID,
		},
	)
	if err != nil {
		return model.OrganizationDomain{}, mapDomainError(
			"DOMAIN_VERIFICATION_UPDATE_FAILED",
			"failed to record domain verification result",
			err,
		)
	}
	activated, err := s.store.Activate(
		ctx,
		organizationID,
		verified.ID,
		false,
		now,
		actorUserID,
	)
	if err != nil {
		return model.OrganizationDomain{}, mapDomainError(
			"DOMAIN_ACTIVATION_FAILED",
			"failed to activate verified domain",
			err,
		)
	}
	return activated, nil
}

func (s *DomainService) RegenerateChallenge(
	ctx context.Context,
	organizationID string,
	domainID string,
	actorUserID string,
) (dto.DomainChallengeResponse, error) {
	if !validUUID(organizationID) || !validUUID(domainID) {
		return dto.DomainChallengeResponse{}, validationError(
			"organization_id and domain_id must be valid UUIDs",
		)
	}
	domain, err := s.store.FindByID(ctx, organizationID, domainID)
	if err != nil {
		return dto.DomainChallengeResponse{}, mapDomainError(
			"DOMAIN_QUERY_FAILED",
			"failed to get organization domain",
			err,
		)
	}
	if domain.Type != model.DomainTypeCustom {
		return dto.DomainChallengeResponse{}, validationError(
			"verification challenge only applies to custom domains",
		)
	}
	if domain.Status == model.DomainStatusActive {
		return dto.DomainChallengeResponse{}, coreerrors.New(
			"DOMAIN_ALREADY_ACTIVE",
			"active domain does not need a new verification challenge",
			http.StatusConflict,
		)
	}
	token, err := s.generateToken(32)
	if err != nil {
		return dto.DomainChallengeResponse{}, coreerrors.Wrap(
			"DOMAIN_CHALLENGE_GENERATION_FAILED",
			"failed to generate domain verification challenge",
			http.StatusInternalServerError,
			err,
		)
	}
	updated, err := s.store.UpdateChallenge(
		ctx,
		organizationID,
		domainID,
		coreauth.HashToken(token),
		s.now().UTC(),
	)
	if err != nil {
		return dto.DomainChallengeResponse{}, mapDomainError(
			"DOMAIN_CHALLENGE_UPDATE_FAILED",
			"failed to update domain verification challenge",
			err,
		)
	}
	return dto.DomainChallengeResponse{
		Domain:        domainResponse(updated),
		ChallengeType: "dns_txt",
		RecordName:    verificationRecordName(updated.CanonicalHost),
		RecordValue:   token,
	}, nil
}

func (s *DomainService) Update(
	ctx context.Context,
	organizationID string,
	domainID string,
	request dto.UpdateDomainRequest,
	actorUserID string,
) (dto.DomainResponse, error) {
	if !validUUID(organizationID) || !validUUID(domainID) {
		return dto.DomainResponse{}, validationError(
			"organization_id and domain_id must be valid UUIDs",
		)
	}
	if request.IsPrimary == nil || !*request.IsPrimary {
		return dto.DomainResponse{}, validationError(
			"is_primary must be true",
		)
	}
	domain, err := s.store.SetPrimary(
		ctx,
		organizationID,
		domainID,
		s.now().UTC(),
		strings.TrimSpace(actorUserID),
	)
	if err != nil {
		return dto.DomainResponse{}, mapDomainError(
			"DOMAIN_PRIMARY_UPDATE_FAILED",
			"failed to set primary organization domain",
			err,
		)
	}
	return domainResponse(domain), nil
}

func (s *DomainService) Delete(
	ctx context.Context,
	organizationID string,
	domainID string,
	actorUserID string,
) error {
	if !validUUID(organizationID) || !validUUID(domainID) {
		return validationError(
			"organization_id and domain_id must be valid UUIDs",
		)
	}
	if err := s.store.Delete(
		ctx,
		organizationID,
		domainID,
		s.now().UTC(),
		strings.TrimSpace(actorUserID),
	); err != nil {
		return mapDomainError(
			"DOMAIN_DELETE_FAILED",
			"failed to disable organization domain",
			err,
		)
	}
	return nil
}

func domainListFilter(
	query dto.DomainListQuery,
) (repository.DomainListFilter, int, int, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	domainType := model.DomainType(strings.TrimSpace(query.Type))
	if domainType != "" &&
		domainType != model.DomainTypeSubdomain &&
		domainType != model.DomainTypeCustom {
		return repository.DomainListFilter{}, 0, 0,
			validationError("domain type is invalid")
	}
	status := model.DomainStatus(strings.TrimSpace(query.Status))
	if status != "" && !status.IsValid() {
		return repository.DomainListFilter{}, 0, 0,
			validationError("domain status is invalid")
	}
	return repository.DomainListFilter{
		Type:           domainType,
		Status:         status,
		IncludeDeleted: query.IncludeDeleted,
		Limit:          perPage,
		Offset:         (page - 1) * perPage,
	}, page, perPage, nil
}

func domainResponse(domain model.OrganizationDomain) dto.DomainResponse {
	return dto.DomainResponse{
		ID:                   domain.ID,
		OrganizationID:       domain.OrganizationID,
		Type:                 string(domain.Type),
		CanonicalHost:        domain.CanonicalHost,
		Status:               string(domain.Status),
		IsPrimary:            domain.IsPrimary,
		VerificationAttempts: domain.VerificationAttempts,
		LastVerificationAt:   formatOrganizationTime(domain.LastVerificationAt),
		VerifiedAt:           formatOrganizationTime(domain.VerifiedAt),
		VerificationError:    domain.VerificationError,
		SSLStatus:            string(domain.SSLStatus),
		SSLError:             domain.SSLError,
		SSLExpiresAt:         formatOrganizationTime(domain.SSLExpiresAt),
		CreatedAt:            domain.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:            domain.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func verificationRecordName(host string) string {
	return domainVerificationPrefix + normalizeDomainHost(host)
}

func normalizeDomainHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func validDomainHost(host string) bool {
	if len(host) < 3 || len(host) > 253 ||
		strings.ContainsAny(host, "/:\\ \t\r\n") ||
		net.ParseIP(host) != nil ||
		!strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !domainLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func mapDomainError(code, message string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New(
			"DOMAIN_NOT_FOUND",
			"organization domain was not found",
			http.StatusNotFound,
		)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == "23505":
			return coreerrors.New(
				"DOMAIN_HOST_CONFLICT",
				"canonical host is already registered",
				http.StatusConflict,
			)
		case pgErr.Code == "23514":
			return validationError("domain data violates a database constraint")
		case pgErr.Code == "ZD001",
			// Fallback untuk DB yang belum menjalankan migration ERRCODE.
			pgErr.Code == "P0001" &&
				strings.Contains(strings.ToLower(pgErr.Message), "reserved"):
			return coreerrors.New(
				"DOMAIN_SUBDOMAIN_RESERVED",
				"subdomain label is reserved",
				http.StatusConflict,
			)
		}
	}
	return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
}
