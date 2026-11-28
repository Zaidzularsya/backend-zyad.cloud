package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	"zyad.cloud/internal/modules/social/service"
)

// visitIPLimitPerMinute bounds visit events per client IP.
const visitIPLimitPerMinute = 60

// PublicAttribution is the subset of the attribution service used without login.
type PublicAttribution interface {
	IsKnownCode(ctx context.Context, code string) (bool, error)
	RecordVisit(ctx context.Context, in service.VisitInput) error
}

// PublicHandler serves referral endpoints that need no account. It never
// returns who owns a code or any customer relation.
type PublicHandler struct {
	svc     PublicAttribution
	counter middleware.RateCounter
}

func NewPublicHandler(svc PublicAttribution, counter middleware.RateCounter) *PublicHandler {
	return &PublicHandler{svc: svc, counter: counter}
}

// RegisterRoutes mounts on the plain api group (no auth, no tenant host).
func (h *PublicHandler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/public/referrals/:code", h.ValidateCode)
	api.POST("/public/attribution/visits",
		middleware.RateLimit(h.counter, "rl:attr:visit:ip:", visitIPLimitPerMinute, time.Minute, func(c *gin.Context) string { return c.ClientIP() }),
		h.RecordVisit,
	)
}

type referralValidityResponse struct {
	Valid bool `json:"valid"`
}

func (h *PublicHandler) ValidateCode(c *gin.Context) {
	valid, err := h.svc.IsKnownCode(c.Request.Context(), c.Param("code"))
	if err != nil {
		failSocial(c, "validate referral code", err)
		return
	}
	corehttp.OK(c, "success", referralValidityResponse{Valid: valid})
}

type recordVisitRequest struct {
	Code    string `json:"code" binding:"required,max=64"`
	Channel string `json:"channel" binding:"max=64"`
	PostID  string `json:"post_id" binding:"max=64"`
}

// RecordVisit answers 204 for known and unknown codes alike.
func (h *PublicHandler) RecordVisit(c *gin.Context) {
	var req recordVisitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	if err := h.svc.RecordVisit(c.Request.Context(), service.VisitInput{
		Code:      req.Code,
		Channel:   req.Channel,
		PostID:    req.PostID,
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
	}); err != nil {
		failSocial(c, "record visit", err)
		return
	}
	corehttp.NoContent(c)
}
