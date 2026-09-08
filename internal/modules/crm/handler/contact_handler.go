package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/shared/response"
)

type ContactHandler struct {
	svc service.ContactService
}

func NewContactHandler(svc service.ContactService) *ContactHandler {
	return &ContactHandler{svc: svc}
}

// RegisterRoutes registers contact routes under the given parent group. See
// CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
//
// "customer" is not a separate base path: the same crm_contacts rows are
// listed here, filtered by ?is_customer=true / ?lifecycle_stage=customer —
// see docs/reference-crm.md "Naming Conflict".
func (h *ContactHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/contacts")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.delete"), h.Delete)
	group.POST("/:id/restore", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.restore"), h.Restore)
}

func (h *ContactHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.ContactListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	contacts, total, err := h.svc.List(c.Request.Context(), scope, repository.ContactListFilter{
		Search:         query.Search,
		CompanyID:      query.CompanyID,
		OwnerUserID:    query.OwnerUserID,
		LifecycleStage: domain.ContactLifecycleStage(query.LifecycleStage),
		IsCustomer:     query.IsCustomer,
		Limit:          perPage,
		Offset:         (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.ContactListFromDomain(contacts), dto.BuildMeta(page, perPage, total))
}

func (h *ContactHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreateContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	lifecycleStage := domain.ContactLifecycleStage(req.LifecycleStage)
	if req.LifecycleStage == "" {
		lifecycleStage = domain.ContactLifecycleContact
	} else if !lifecycleStage.IsValid() {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid lifecycle_stage", http.StatusUnprocessableEntity))
		return
	}

	contact, err := h.svc.Create(c.Request.Context(), scope, repository.CreateContactParams{
		CompanyID:      req.CompanyID,
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		Email:          req.Email,
		Phone:          req.Phone,
		JobTitle:       req.JobTitle,
		Address:        req.Address,
		Tags:           req.Tags,
		Source:         req.Source,
		OwnerUserID:    req.OwnerUserID,
		IsCustomer:     lifecycleStage == domain.ContactLifecycleCustomer,
		LifecycleStage: lifecycleStage,
		CreatedBy:      userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ContactFromDomain(contact))
}

func (h *ContactHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	contact, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ContactFromDomain(contact))
}

func (h *ContactHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var lifecycleStage *domain.ContactLifecycleStage
	if req.LifecycleStage != nil {
		stage := domain.ContactLifecycleStage(*req.LifecycleStage)
		if !stage.IsValid() {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid lifecycle_stage", http.StatusUnprocessableEntity))
			return
		}
		lifecycleStage = &stage
	}

	contact, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), repository.UpdateContactParams{
		CompanyID:      req.CompanyID,
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		Email:          req.Email,
		Phone:          req.Phone,
		JobTitle:       req.JobTitle,
		Address:        req.Address,
		Tags:           req.Tags,
		Source:         req.Source,
		OwnerUserID:    req.OwnerUserID,
		IsCustomer:     req.IsCustomer,
		LifecycleStage: lifecycleStage,
		UpdatedBy:      userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ContactFromDomain(contact))
}

func (h *ContactHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	if err := h.svc.Delete(c.Request.Context(), scope, c.Param("id"), userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func (h *ContactHandler) Restore(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	if err := h.svc.Restore(c.Request.Context(), scope, c.Param("id"), userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "restored", nil)
}
