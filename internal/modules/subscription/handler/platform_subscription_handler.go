package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/subscription/dto"
	"zyad.cloud/internal/modules/subscription/model"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func validationHandlerError(err error) error {
	return coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity)
}

type PlatformSubscriptionService interface {
	ListAllOrganizations(
		ctx context.Context,
		query dto.SubscriptionListQuery,
	) (dto.SubscriptionListResponse, error)
	Create(ctx context.Context, request dto.CreateSubscriptionRequest) (dto.SubscriptionResponse, error)
	UpdateByID(ctx context.Context, id string, request dto.UpdateSubscriptionRequest) (dto.SubscriptionResponse, error)
	ChangeStatusByID(
		ctx context.Context,
		id string,
		status model.SubscriptionStatus,
		actorUserID string,
		reason string,
	) (dto.SubscriptionResponse, error)
}

type PlatformSubscriptionHandler struct {
	subscriptions PlatformSubscriptionService
	checker       permissionmiddleware.PermissionChecker
}

func NewPlatformSubscriptionHandler(
	subscriptions PlatformSubscriptionService,
	checker permissionmiddleware.PermissionChecker,
) *PlatformSubscriptionHandler {
	return &PlatformSubscriptionHandler{
		subscriptions: subscriptions,
		checker:       checker,
	}
}

func (h *PlatformSubscriptionHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/subscriptions")
	group.Use(middleware.RequirePlatformTenant())

	group.GET("", permissionmiddleware.Require(h.checker, "platform.subscription.read"), h.ListSubscriptions)
	group.POST("", permissionmiddleware.Require(h.checker, "platform.subscription.manage"), h.CreateSubscription)
	group.PATCH("/:id", permissionmiddleware.Require(h.checker, "platform.subscription.manage"), h.UpdateSubscription)
	group.POST("/:id/cancel", permissionmiddleware.Require(h.checker, "platform.subscription.manage"), h.CancelSubscription)
	group.POST("/:id/suspend", permissionmiddleware.Require(h.checker, "platform.subscription.manage"), h.SuspendSubscription)
}

func (h *PlatformSubscriptionHandler) ListSubscriptions(c *gin.Context) {
	var query dto.SubscriptionListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.ListAllOrganizations(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "billing subscriptions retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformSubscriptionHandler) CreateSubscription(c *gin.Context) {
	var request dto.CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.Create(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "billing subscription created successfully", result)
}

func (h *PlatformSubscriptionHandler) UpdateSubscription(c *gin.Context) {
	var request dto.UpdateSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.UpdateByID(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing subscription updated successfully", result)
}

func (h *PlatformSubscriptionHandler) CancelSubscription(c *gin.Context) {
	reason := struct {
		Reason string `json:"reason"`
	}{}
	if err := c.ShouldBindJSON(&reason); err != nil && !errors.Is(err, io.EOF) {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.ChangeStatusByID(
		c.Request.Context(),
		c.Param("id"),
		model.SubscriptionStatusCanceled,
		permissionmiddleware.UserID(c),
		reason.Reason,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing subscription canceled successfully", result)
}

func (h *PlatformSubscriptionHandler) SuspendSubscription(c *gin.Context) {
	reason := struct {
		Reason string `json:"reason"`
	}{}
	if err := c.ShouldBindJSON(&reason); err != nil && !errors.Is(err, io.EOF) {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.ChangeStatusByID(
		c.Request.Context(),
		c.Param("id"),
		model.SubscriptionStatusSuspended,
		permissionmiddleware.UserID(c),
		reason.Reason,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing subscription suspended successfully", result)
}
