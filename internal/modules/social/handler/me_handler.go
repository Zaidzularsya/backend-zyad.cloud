package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	"zyad.cloud/internal/modules/social/domain"
	"zyad.cloud/internal/modules/social/service"
)

type MeAttribution interface {
	Summary(ctx context.Context, userID string) (service.Summary, error)
}

type MeRelations interface {
	ForUser(ctx context.Context, userID string) ([]service.CustomerRelation, error)
}

type MeClaims interface {
	Offers(ctx context.Context, user service.AuthUser) ([]service.ClaimOffer, error)
	Accept(ctx context.Context, user service.AuthUser, organizationID, contactID string) (domain.CustomerLink, error)
	Decline(ctx context.Context, user service.AuthUser, organizationID, contactID string) error
	Revoke(ctx context.Context, actorUserID, linkID string, byStaff bool) error
}

type MeInvites interface {
	Accept(ctx context.Context, user service.AuthUser, token string) (domain.CustomerLink, error)
}

// MeHandler serves the signed-in user's own social data. It needs no tenant
// context: everything is keyed by the authenticated user.
type MeHandler struct {
	attribution MeAttribution
	relations   MeRelations
	claims      MeClaims
	invites     MeInvites
	counter     middleware.RateCounter
}

// All /social/me writes share one per-user bucket (spec §6.12).
const (
	meWriteUserLimit  = 20
	meWriteUserWindow = time.Minute
)

func NewMeHandler(attribution MeAttribution, relations MeRelations, claims MeClaims, invites MeInvites, counter middleware.RateCounter) *MeHandler {
	return &MeHandler{attribution: attribution, relations: relations, claims: claims, invites: invites, counter: counter}
}

// RegisterRoutes mounts on the protected group.
func (h *MeHandler) RegisterRoutes(protected *gin.RouterGroup) {
	group := protected.Group("/social/me")
	group.GET("/attribution", h.Attribution)
	group.GET("/customer-relations", h.CustomerRelations)
	perUser := middleware.RateLimit(h.counter, "rl:social:me:user:", meWriteUserLimit, meWriteUserWindow,
		func(c *gin.Context) string {
			if user, ok := middleware.AuthenticatedUserFromContext(c); ok && user.ID != "" {
				return user.ID
			}
			return "unknown"
		})
	if h.claims != nil {
		verified := middleware.RequireVerifiedEmail()
		group.GET("/customer-claims", verified, h.CustomerClaims)
		group.POST("/customer-claims/accept", perUser, verified, h.AcceptCustomerClaim)
		group.POST("/customer-claims/decline", perUser, verified, h.DeclineCustomerClaim)
		// No RequireVerifiedEmail: a user can always undo their own link.
		group.DELETE("/customer-links/:id", perUser, h.RevokeCustomerLink)
	}
	if h.invites != nil {
		// No RequireVerifiedEmail: the emailed token is itself the proof of
		// email ownership and may verify the account.
		group.POST("/customer-invites/accept", perUser, h.AcceptCustomerInvite)
	}
}

type attributionSummaryResponse struct {
	Code     string `json:"code"`
	ShareURL string `json:"share_url"`
	Visits   int    `json:"visits"`
	Signups  int    `json:"signups"`
}

func (h *MeHandler) Attribution(c *gin.Context) {
	user, ok := requireUser(c)
	if !ok {
		return
	}
	summary, err := h.attribution.Summary(c.Request.Context(), user.ID)
	if err != nil {
		failSocial(c, "attribution summary", err)
		return
	}
	corehttp.OK(c, "success", attributionSummaryResponse{
		Code: summary.Code, ShareURL: summary.ShareURL, Visits: summary.Visits, Signups: summary.Signups,
	})
}

type customerRelationResponse struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	Source           string `json:"source"`
	CustomerLinkID   string `json:"customer_link_id,omitempty"`
}

func (h *MeHandler) CustomerRelations(c *gin.Context) {
	user, ok := requireUser(c)
	if !ok {
		return
	}
	relations, err := h.relations.ForUser(c.Request.Context(), user.ID)
	if err != nil {
		failSocial(c, "customer relations", err)
		return
	}
	out := make([]customerRelationResponse, 0, len(relations))
	for _, r := range relations {
		out = append(out, customerRelationResponse{
			OrganizationID: r.OrganizationID, OrganizationName: r.OrganizationName,
			Source: r.Source, CustomerLinkID: r.CustomerLinkID,
		})
	}
	corehttp.OK(c, "success", gin.H{"items": out})
}

type customerClaimOfferResponse struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	ContactID        string `json:"contact_id"`
	ContactName      string `json:"contact_name"`
	Kind             string `json:"kind"`
	LinkID           string `json:"link_id,omitempty"`
}

type customerClaimRequest struct {
	OrganizationID string `json:"organization_id" binding:"required,uuid"`
	ContactID      string `json:"contact_id" binding:"required,uuid"`
}

type customerLinkResponse struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	ContactID      string     `json:"contact_id"`
	Status         string     `json:"status"`
	VerifiedVia    string     `json:"verified_via"`
	ActivatedAt    *time.Time `json:"activated_at"`
}

func authUser(user middleware.AuthenticatedUser) service.AuthUser {
	return service.AuthUser{ID: user.ID, Email: user.Email, EmailVerified: user.EmailVerified}
}

func (h *MeHandler) CustomerClaims(c *gin.Context) {
	user, ok := requireUser(c)
	if !ok {
		return
	}
	offers, err := h.claims.Offers(c.Request.Context(), authUser(user))
	if err != nil {
		failSocial(c, "customer claim offers", err)
		return
	}
	out := make([]customerClaimOfferResponse, 0, len(offers))
	for _, o := range offers {
		out = append(out, customerClaimOfferResponse{
			OrganizationID: o.OrganizationID, OrganizationName: o.OrganizationName,
			ContactID: o.ContactID, ContactName: o.ContactName, Kind: o.Kind, LinkID: o.LinkID,
		})
	}
	corehttp.OK(c, "success", gin.H{"items": out})
}

func (h *MeHandler) AcceptCustomerClaim(c *gin.Context) {
	user, ok := requireUser(c)
	if !ok {
		return
	}
	req, ok := bindClaimRequest(c)
	if !ok {
		return
	}
	link, err := h.claims.Accept(c.Request.Context(), authUser(user), req.OrganizationID, req.ContactID)
	if err != nil {
		failSocial(c, "accept customer claim", err)
		return
	}
	corehttp.OK(c, "success", customerLinkResponse{
		ID: link.ID, OrganizationID: link.OrganizationID, ContactID: link.ContactID,
		Status: link.Status, VerifiedVia: link.VerifiedVia, ActivatedAt: link.ActivatedAt,
	})
}

type acceptCustomerInviteRequest struct {
	Token string `json:"token" binding:"required,max=256"`
}

func (h *MeHandler) AcceptCustomerInvite(c *gin.Context) {
	user, ok := requireUser(c)
	if !ok {
		return
	}
	var req acceptCustomerInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	link, err := h.invites.Accept(c.Request.Context(), authUser(user), req.Token)
	if err != nil {
		failSocial(c, "accept customer claim invite", err)
		return
	}
	corehttp.OK(c, "success", customerLinkResponse{
		ID: link.ID, OrganizationID: link.OrganizationID, ContactID: link.ContactID,
		Status: link.Status, VerifiedVia: link.VerifiedVia, ActivatedAt: link.ActivatedAt,
	})
}

func (h *MeHandler) DeclineCustomerClaim(c *gin.Context) {
	user, ok := requireUser(c)
	if !ok {
		return
	}
	req, ok := bindClaimRequest(c)
	if !ok {
		return
	}
	if err := h.claims.Decline(c.Request.Context(), authUser(user), req.OrganizationID, req.ContactID); err != nil {
		failSocial(c, "decline customer claim", err)
		return
	}
	corehttp.NoContent(c)
}

func (h *MeHandler) RevokeCustomerLink(c *gin.Context) {
	user, ok := requireUser(c)
	if !ok {
		return
	}
	linkID := c.Param("id")
	if !isUUID(linkID) {
		failSocial(c, "revoke customer link", domain.ErrCustomerLinkNotFound)
		return
	}
	if err := h.claims.Revoke(c.Request.Context(), user.ID, linkID, false); err != nil {
		failSocial(c, "revoke customer link", err)
		return
	}
	corehttp.NoContent(c)
}

func bindClaimRequest(c *gin.Context) (customerClaimRequest, bool) {
	var req customerClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return customerClaimRequest{}, false
	}
	return req, true
}
