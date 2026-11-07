package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/businesstime"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/shared/response"
)

// Public isolir page limits. The per-query bucket stops one identifier from
// being enumerated from many IPs; the per-tenant bucket caps total load.
const (
	isolirIPLimit     = 20
	isolirQueryLimit  = 5
	isolirTenantLimit = 300
	isolirWindow      = time.Minute
	// isolirMaxBodyBytes bounds the read; the service enforces the 100 char query cap.
	isolirMaxBodyBytes = 2048

	isolirQueryCtxKey = "mikrotik.isolir.query"
)

// PublicService is the public isolir page use case (service.PublicService).
type PublicService interface {
	Info(ctx context.Context, scope coretenant.Scope) (service.PublicInfo, error)
	Lookup(ctx context.Context, scope coretenant.Scope, query string, today time.Time) (service.PublicLookupResult, error)
}

// IsolirPublicHandler serves /public/isolir/*. It MUST be mounted on the
// host-resolved public group: the tenant comes from the host only.
type IsolirPublicHandler struct {
	svc     PublicService
	counter middleware.RateCounter
	now     func() time.Time
}

func NewIsolirPublicHandler(svc PublicService, counter middleware.RateCounter) *IsolirPublicHandler {
	return &IsolirPublicHandler{svc: svc, counter: counter, now: time.Now}
}

func (h *IsolirPublicHandler) RegisterRoutes(router gin.IRoutes) {
	byIP := middleware.RateLimit(h.counter, "rl:isolir:ip:", isolirIPLimit, isolirWindow,
		func(c *gin.Context) string { return c.ClientIP() })
	byTenant := middleware.RateLimit(h.counter, "rl:isolir:org:", isolirTenantLimit, isolirWindow, isolirTenantKey)
	byQuery := middleware.RateLimit(h.counter, "rl:isolir:q:", isolirQueryLimit, isolirWindow, func(c *gin.Context) string {
		q, _ := c.Get(isolirQueryCtxKey)
		s, _ := q.(string)
		// Hash only: the raw identifier (phone/username) never reaches Redis keys or logs.
		sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(s))))
		return isolirTenantKey(c) + ":" + hex.EncodeToString(sum[:16])
	})
	router.GET("/public/isolir/info", noStoreNoIndex, byIP, byTenant, h.Info)
	router.POST("/public/isolir/lookup", noStoreNoIndex, readLookupQuery, byIP, byQuery, byTenant, h.Lookup)
}

func isolirTenantKey(c *gin.Context) string {
	if tc, ok := coretenant.FromContext(c.Request.Context()); ok {
		return tc.OrganizationID()
	}
	return "unknown"
}

func noStoreNoIndex(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex")
	c.Next()
}

// readLookupQuery extracts the query with a bounded read. Any problem (too
// large, malformed, wrong type, missing field) yields an empty query so the
// endpoint answers uniformly instead of acting as a probing oracle.
func readLookupQuery(c *gin.Context) {
	var body dto.IsolirLookupRequest
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, isolirMaxBodyBytes+1))
	if err == nil && len(raw) <= isolirMaxBodyBytes {
		if json.Unmarshal(raw, &body) != nil {
			body = dto.IsolirLookupRequest{}
		}
	}
	c.Set(isolirQueryCtxKey, body.Query)
	c.Next()
}

func isolirFail(c *gin.Context, op string, err error) {
	// Never log the query or token; the cause class is enough to debug.
	slog.Error("public isolir failed", "op", op, "error", err)
	response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func (h *IsolirPublicHandler) Info(c *gin.Context) {
	scope, err := publicScope(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	info, err := h.svc.Info(c.Request.Context(), scope)
	if err != nil {
		isolirFail(c, "info", err)
		return
	}
	corehttp.OK(c, "ok", dto.IsolirInfoFromService(info))
}

func (h *IsolirPublicHandler) Lookup(c *gin.Context) {
	scope, err := publicScope(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	q, _ := c.Get(isolirQueryCtxKey)
	query, _ := q.(string)
	res, err := h.svc.Lookup(c.Request.Context(), scope, query, businesstime.DayOf(h.now()))
	if err != nil {
		isolirFail(c, "lookup", err)
		return
	}
	corehttp.OK(c, "ok", dto.IsolirLookupFromService(res))
}

func publicScope(c *gin.Context) (coretenant.Scope, error) {
	tc, err := coretenant.RequireContext(c.Request.Context())
	if err != nil {
		return coretenant.Scope{}, err
	}
	return coretenant.NewScope(tc)
}
