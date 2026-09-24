package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	whatsappmodule "zyad.cloud/internal/modules/whatsapp"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/dto"
	"zyad.cloud/internal/modules/whatsapp/service"
	"zyad.cloud/internal/shared/response"
)

type ConversationHandler struct {
	svc         *service.ConversationService
	permissions permissionmiddleware.CombinedPermissionChecker
}

func NewConversationHandler(svc *service.ConversationService) *ConversationHandler {
	return &ConversationHandler{svc: svc}
}

// RegisterRoutes mounts conversation and message routes under /app/whatsapp.
// Route permissions gate the action; visibility (own vs read_all) and
// reassignment (assign) are enforced by the service through the Viewer.
func (h *ConversationHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	h.permissions = p
	read := permissionmiddleware.RequireOrganizationOrGlobal(p, domain.PermissionConversationRead)
	send := permissionmiddleware.RequireOrganizationOrGlobal(p, domain.PermissionMessageSend)

	group := router.Group("/conversations")
	group.GET("", read, h.List)
	group.POST("/start", send, h.Start)
	group.GET("/:id", read, h.Get)
	group.PATCH("/:id", read, h.Update)
	group.POST("/:id/read", read, h.MarkRead)
	group.GET("/:id/messages", read, h.Messages)
	group.POST("/:id/messages", send, h.Send)
	router.POST("/messages/:id/retry", send, h.Retry)
}

// viewer resolves the caller's read_all/assign grants with the same
// organization-then-global rule as RequireOrganizationOrGlobal.
func (h *ConversationHandler) viewer(c *gin.Context, scope coretenant.Scope) service.Viewer {
	userID := permissionmiddleware.UserID(c)
	has := func(permission string) bool {
		if h.permissions == nil || userID == "" {
			return false
		}
		required := []string{permission}
		if h.permissions.CanOrganization(c.Request.Context(), userID, scope.OrganizationID(), required) == nil {
			return true
		}
		return h.permissions.Can(c.Request.Context(), userID, required) == nil
	}
	return service.Viewer{
		UserID:     userID,
		CanReadAll: has(domain.PermissionConversationReadAll),
		CanAssign:  has(domain.PermissionConversationAssign),
	}
}

func (h *ConversationHandler) List(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var query dto.ConversationListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		failValidation(c, err)
		return
	}
	entityType := domain.RelatedEntityType(query.RelatedEntityType)
	if query.RelatedEntityType != "" && !entityType.IsValid() {
		corehttp.Fail(c, whatsappmodule.ErrInvalidEntityType)
		return
	}
	page, perPage := query.Page, query.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 100 {
		perPage = 20
	}
	conversations, total, err := h.svc.List(c.Request.Context(), scope, h.viewer(c, scope), service.ConversationListInput{
		RelatedEntityType: entityType,
		RelatedEntityID:   query.RelatedEntityID,
		AssigneeUserID:    query.Assignee,
		Status:            domain.ConversationStatus(query.Status),
		Search:            query.Search,
		Limit:             perPage,
		Offset:            (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "success", dto.ConversationListFromDomain(conversations), dto.BuildMeta(page, perPage, total))
}

func (h *ConversationHandler) Get(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	conversation, err := h.svc.Get(c.Request.Context(), scope, h.viewer(c, scope), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ConversationFromDomain(conversation))
}

func (h *ConversationHandler) Start(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var req dto.StartConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failValidation(c, err)
		return
	}
	conversation, err := h.svc.Start(c.Request.Context(), scope, h.viewer(c, scope), service.StartConversationInput{
		SessionID:         req.SessionID,
		RelatedEntityType: domain.RelatedEntityType(req.RelatedEntityType),
		RelatedEntityID:   req.RelatedEntityID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ConversationFromDomain(conversation))
}

func (h *ConversationHandler) Update(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var req dto.UpdateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failValidation(c, err)
		return
	}
	input := service.UpdateConversationInput{AssigneeUserID: req.AssigneeUserID}
	if req.Status != nil {
		status := domain.ConversationStatus(*req.Status)
		input.Status = &status
	}
	conversation, err := h.svc.Update(c.Request.Context(), scope, h.viewer(c, scope), c.Param("id"), input)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "updated", dto.ConversationFromDomain(conversation))
}

func (h *ConversationHandler) MarkRead(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), scope, h.viewer(c, scope), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", nil)
}

func (h *ConversationHandler) Messages(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var query dto.MessageListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		failValidation(c, err)
		return
	}
	page, err := h.svc.Messages(c.Request.Context(), scope, h.viewer(c, scope), c.Param("id"), query.Before, query.Limit)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.MessagePageFromDomain(page.Messages, page.NextBefore))
}

func (h *ConversationHandler) Send(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var req dto.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failValidation(c, err)
		return
	}
	message, err := h.svc.Send(c.Request.Context(), scope, h.viewer(c, scope), c.Param("id"), req.Text)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "created", dto.MessageFromDomain(message))
}

func (h *ConversationHandler) Retry(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	message, err := h.svc.Retry(c.Request.Context(), scope, h.viewer(c, scope), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.MessageFromDomain(message))
}
