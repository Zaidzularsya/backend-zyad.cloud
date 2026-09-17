package handler

import (
	"context"
	"net/http"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type JournalService interface {
	Create(ctx context.Context, request dto.CreateJournalEntryRequest, actorID string) (dto.JournalEntryResponse, error)
	Get(ctx context.Context, id string) (dto.JournalEntryResponse, error)
	List(ctx context.Context, query dto.JournalEntryListQuery) (dto.JournalEntryListResponse, error)
	Reverse(ctx context.Context, id string, request dto.ReverseJournalEntryRequest, actorID string) (dto.JournalEntryResponse, error)
}

type PlatformJournalHandler struct {
	journal JournalService
	checker permissionmiddleware.PermissionChecker
}

func NewPlatformJournalHandler(journal JournalService, checker permissionmiddleware.PermissionChecker) *PlatformJournalHandler {
	return &PlatformJournalHandler{journal: journal, checker: checker}
}

func (h *PlatformJournalHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/finance")
	group.Use(middleware.RequirePlatformTenant())

	read := permissionmiddleware.Require(h.checker, "platform.finance.journal.read")
	manage := permissionmiddleware.Require(h.checker, "platform.finance.journal.manage")

	group.GET("/journal-entries", read, h.List)
	group.GET("/journal-entries/:id", read, h.Get)
	group.POST("/journal-entries", manage, h.Create)
	group.POST("/journal-entries/:id/reverse", manage, h.Reverse)
}

func (h *PlatformJournalHandler) List(c *gin.Context) {
	var query dto.JournalEntryListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.journal.List(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "journal entries retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformJournalHandler) Get(c *gin.Context) {
	result, err := h.journal.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "journal entry retrieved successfully", result)
}

func (h *PlatformJournalHandler) Create(c *gin.Context) {
	var request dto.CreateJournalEntryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.journal.Create(c.Request.Context(), request, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "journal entry created successfully", result)
}

func (h *PlatformJournalHandler) Reverse(c *gin.Context) {
	var request dto.ReverseJournalEntryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.journal.Reverse(c.Request.Context(), c.Param("id"), request, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "journal entry reversed successfully", result)
}
