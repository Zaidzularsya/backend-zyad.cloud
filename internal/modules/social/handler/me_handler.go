package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/modules/social/service"
)

type MeAttribution interface {
	Summary(ctx context.Context, userID string) (service.Summary, error)
}

type MeRelations interface {
	ForUser(ctx context.Context, userID string) ([]service.CustomerRelation, error)
}

// MeHandler serves the signed-in user's own social data. It needs no tenant
// context: everything is keyed by the authenticated user.
type MeHandler struct {
	attribution MeAttribution
	relations   MeRelations
}

func NewMeHandler(attribution MeAttribution, relations MeRelations) *MeHandler {
	return &MeHandler{attribution: attribution, relations: relations}
}

// RegisterRoutes mounts on the protected group.
func (h *MeHandler) RegisterRoutes(protected *gin.RouterGroup) {
	group := protected.Group("/social/me")
	group.GET("/attribution", h.Attribution)
	group.GET("/customer-relations", h.CustomerRelations)
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
