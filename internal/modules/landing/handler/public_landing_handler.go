package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

type PublicLandingHandler struct {
	resolverSvc   service.ResolverService
	visibilitySvc service.VisibilityService
	submissionSvc service.SubmissionService
	analyticsSvc  service.AnalyticsService
}

func NewPublicLandingHandler(
	resolverSvc service.ResolverService,
	visibilitySvc service.VisibilityService,
	submissionSvc service.SubmissionService,
	analyticsSvc service.AnalyticsService,
) *PublicLandingHandler {
	return &PublicLandingHandler{
		resolverSvc:   resolverSvc,
		visibilitySvc: visibilitySvc,
		submissionSvc: submissionSvc,
		analyticsSvc:  analyticsSvc,
	}
}

func (h *PublicLandingHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/public/landing")

	group.GET("/resolve", h.Resolve)
	group.GET("/preview/:token", h.Preview)
	group.POST("/access/:publicPageId", h.RequestAccess)
	group.POST("/forms/:formKey/submissions", h.SubmitForm)
	group.POST("/forms/:formKey/uploads", h.UploadFile)
	group.POST("/events", h.TrackEvent)
}

func (h *PublicLandingHandler) Resolve(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	slug := c.Query("slug")
	// If custom domain is passed in Host or specific header:
	customDomain := c.Request.Host
	// Support x-forwarded-host if running behind proxy
	if forwardedHost := c.GetHeader("X-Forwarded-Host"); forwardedHost != "" {
		customDomain = forwardedHost
	}

	var resolved service.ResolvedPage
	var resolveErr error

	// Priority: Domain first, then slug
	if customDomain != "" && customDomain != "localhost" && customDomain != "localhost:8080" { // simple check
		resolved, resolveErr = h.resolverSvc.ResolveByDomain(c.Request.Context(), scope, customDomain, "")
		// Fallback to slug if domain not found and slug is provided
		if resolveErr != nil && slug != "" {
			resolved, resolveErr = h.resolverSvc.ResolveBySlug(c.Request.Context(), scope, slug, "")
		}
	} else if slug != "" {
		resolved, resolveErr = h.resolverSvc.ResolveBySlug(c.Request.Context(), scope, slug, "")
	} else {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "Slug or valid domain is required", http.StatusBadRequest))
		return
	}

	if resolveErr != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", resolveErr.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "Page resolved successfully", resolved)
}

func (h *PublicLandingHandler) Preview(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	token := c.Param("token")
	slug := c.Query("slug")

	if token == "" {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "Token is required", http.StatusBadRequest))
		return
	}

	customDomain := c.Request.Host
	if forwardedHost := c.GetHeader("X-Forwarded-Host"); forwardedHost != "" {
		customDomain = forwardedHost
	}

	var resolved service.ResolvedPage
	var resolveErr error

	if customDomain != "" && customDomain != "localhost" && customDomain != "localhost:8080" {
		resolved, resolveErr = h.resolverSvc.ResolveByDomain(c.Request.Context(), scope, customDomain, token)
		if resolveErr != nil && slug != "" {
			resolved, resolveErr = h.resolverSvc.ResolveBySlug(c.Request.Context(), scope, slug, token)
		}
	} else if slug != "" {
		resolved, resolveErr = h.resolverSvc.ResolveBySlug(c.Request.Context(), scope, slug, token)
	} else {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "Slug or valid domain is required", http.StatusBadRequest))
		return
	}

	if resolveErr != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", resolveErr.Error(), http.StatusInternalServerError))
		return
	}

	// Set required headers for preview
	c.Header("X-Robots-Tag", "noindex, nofollow")
	c.Header("Cache-Control", "no-store")

	corehttp.OK(c, "Preview resolved successfully", resolved)
}

func (h *PublicLandingHandler) RequestAccess(c *gin.Context) {
	pageID := c.Param("publicPageId")

	var req dto.PublicAccessChallengeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	ipAddress := c.ClientIP()

	token, err := h.visibilitySvc.VerifyAccess(c.Request.Context(), "", pageID, req.Password, ipAddress)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "Access granted", gin.H{"access_token": token})
}

func (h *PublicLandingHandler) SubmitForm(c *gin.Context) {
	formKey := c.Param("formKey")

	var req dto.PublicSubmissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	if req.Website != "" {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid submission", http.StatusUnprocessableEntity))
		return
	}

	// Verify idempotency
	idempotencyKey := c.GetHeader("Idempotency-Key")

	// The PageID is not directly in the payload, wait, how do we know the page?
	// It's usually part of the context or we resolve from FormKey.
	// For MVP, we will leave PageID blank if it's not present, or rely on form lookup.
	// Actually we should look up the form to get the page ID, but form key is unique per page or globally?
	// The repository params:
	params := repository.CreateSubmissionParams{
		FormID:         formKey,
		LandingPageID:  "", // Form lookup should resolve this in a real scenario
		Status:         domain.SubmissionStatusNew,
		SubmittedData:  req.Fields,
		SourceURL:      "", // from context if any
		Referrer:       req.Context.Referrer,
		UTMSource:      req.Context.UTMSource,
		UTMMedium:      req.Context.UTMMedium,
		UTMCampaign:    req.Context.UTMCampaign,
		UTMTerm:        req.Context.UTMTerm,
		UTMContent:     req.Context.UTMContent,
		IPAddressHash:  c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
		IdempotencyKey: idempotencyKey,
	}

	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	submission, err := h.submissionSvc.SubmitForm(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "Form submitted successfully", submission)
}

func (h *PublicLandingHandler) UploadFile(c *gin.Context) {
	corehttp.Fail(c, coreerrors.New("NOT_IMPLEMENTED", "File upload is not yet implemented", http.StatusNotImplemented))
}

func (h *PublicLandingHandler) TrackEvent(c *gin.Context) {
	var req dto.PublicAnalyticsEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.Request.UserAgent()

	// Parse Context from the request
	var utmSource, utmMedium, utmCampaign, utmTerm, utmContent, referrer, sessionID string
	if req.Context != nil {
		if v, ok := req.Context["utm_source"].(string); ok {
			utmSource = v
		}
		if v, ok := req.Context["utm_medium"].(string); ok {
			utmMedium = v
		}
		if v, ok := req.Context["utm_campaign"].(string); ok {
			utmCampaign = v
		}
		if v, ok := req.Context["utm_term"].(string); ok {
			utmTerm = v
		}
		if v, ok := req.Context["utm_content"].(string); ok {
			utmContent = v
		}
		if v, ok := req.Context["referrer"].(string); ok {
			referrer = v
		}
	}
	sessionID = req.SessionID

	params := service.TrackEventParams{
		LandingPageID: req.PageID,
		Version:       req.PageVersion,
		EventName:     domain.AnalyticsEventName(req.Event),
		SectionKey:    &req.SectionKey,
		TargetKey:     &req.TargetKey,
		SessionID:     &sessionID,
		UTMSource:     &utmSource,
		UTMMedium:     &utmMedium,
		UTMCampaign:   &utmCampaign,
		UTMTerm:       &utmTerm,
		UTMContent:    &utmContent,
		Referrer:      &referrer,
		Browser:       &userAgent,
		IPAddress:     &ipAddress,
	}

	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	err = h.analyticsSvc.TrackEvent(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "Event tracked successfully", nil)
}
