package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

type AdminFormHandler struct {
	formSvc service.FormService
}

func NewAdminFormHandler(formSvc service.FormService) *AdminFormHandler {
	return &AdminFormHandler{
		formSvc: formSvc,
	}
}

func (h *AdminFormHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.CombinedPermissionChecker) {
	pageGroup := router.Group("/admin/landing-pages/:id/forms")

	pageGroup.GET("", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.page.read"), h.ListForms)
	pageGroup.POST("", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.form.manage"), h.CreateForm)
	pageGroup.PATCH("/:formId", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.form.manage"), h.UpdateForm)
	pageGroup.DELETE("/:formId", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.form.manage"), h.DeleteForm)
	pageGroup.PUT("/:formId/fields", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.form.manage"), h.ReplaceFields)
}

func (h *AdminFormHandler) ListForms(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")

	forms, err := h.formSvc.ListFormsByPage(c.Request.Context(), scope, pageID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Forms retrieved successfully", forms)
}

func (h *AdminFormHandler) CreateForm(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")

	var req dto.CreateFormRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var redirectURL string
	if req.RedirectURL != nil {
		redirectURL = *req.RedirectURL
		if err := dto.ValidateRedirectURL(redirectURL); err != nil {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
			return
		}
	}

	// SuccessMessage fallback
	var successMsg string = req.SuccessMessage
	if successMsg == "" {
		successMsg = "Success"
	}

	// A new form creates CRM leads unless explicitly turned off.
	createCRMLead := true
	if req.CreateCRMLead != nil {
		createCRMLead = *req.CreateCRMLead
	}
	var leadOwner string
	if req.LeadOwnerUserID != nil {
		leadOwner = *req.LeadOwnerUserID
	}

	params := repository.CreateFormParams{
		AfterSubmit:     afterSubmitFromRequest(req.AfterSubmit),
		WhatsAppNumber:  derefString(req.WhatsAppNumber),
		WhatsAppMessage: derefString(req.WhatsAppMessage),
		LandingPageID:   pageID,
		Name:            req.Name,
		Key:             req.Key,
		SubmitLabel:     req.SubmitLabel,
		SuccessMessage:  successMsg,
		RedirectURL:     redirectURL,
		IsActive:        req.IsActive,
		CreatedBy:       permissionmiddleware.UserID(c),

		CreateCRMLead:   createCRMLead,
		LeadOwnerUserID: leadOwner,
	}

	form, err := h.formSvc.CreateForm(c.Request.Context(), scope, params)
	if err != nil {
		failFormWrite(c, err)
		return
	}

	corehttp.OK(c, "Form created successfully", form)
}

func (h *AdminFormHandler) UpdateForm(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	formID := c.Param("formId")

	var req dto.UpdateFormRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	if req.RedirectURL != nil {
		if err := dto.ValidateRedirectURL(*req.RedirectURL); err != nil {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
			return
		}
	}

	params := repository.UpdateFormParams{
		Name:           req.Name,
		SubmitLabel:    req.SubmitLabel,
		SuccessMessage: req.SuccessMessage,
		RedirectURL:    req.RedirectURL,
		IsActive:       req.IsActive,
		UpdatedBy:      permissionmiddleware.UserID(c),

		CreateCRMLead:   req.CreateCRMLead,
		LeadOwnerUserID: req.LeadOwnerUserID,
		WhatsAppNumber:  req.WhatsAppNumber,
		WhatsAppMessage: req.WhatsAppMessage,
	}
	if req.AfterSubmit != nil {
		action := domain.AfterSubmitAction(*req.AfterSubmit)
		params.AfterSubmit = &action
	}

	form, err := h.formSvc.UpdateForm(c.Request.Context(), scope, formID, params)
	if err != nil {
		failFormWrite(c, err)
		return
	}

	corehttp.OK(c, "Form updated successfully", form)
}

func (h *AdminFormHandler) DeleteForm(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	formID := c.Param("formId")

	if err := h.formSvc.DeleteForm(c.Request.Context(), scope, formID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Form deleted successfully", nil)
}

func (h *AdminFormHandler) ReplaceFields(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	formID := c.Param("formId")

	var req dto.ReplaceFormFieldsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var params []repository.CreateFormFieldParams
	for _, f := range req.Fields {
		params = append(params, repository.CreateFormFieldParams{
			FormID:      formID,
			Key:         f.Key,
			Type:        domain.FormFieldType(f.Type),
			Label:       f.Label,
			Placeholder: f.Placeholder,
			Options:     f.Options,
			Validation:  f.Validation,
			IsRequired:  f.Required,
			SortOrder:   f.SortOrder,

			DefaultValue: f.DefaultValue,
		})
	}

	fields, err := h.formSvc.ReplaceFields(c.Request.Context(), scope, formID, params)
	if err != nil {
		failFormWrite(c, err)
		return
	}

	corehttp.OK(c, "Form fields replaced successfully", fields)
}

const coreFieldsMessage = "Nama, Email, dan No. WA wajib untuk form yang meneruskan ke CRM."

// failFormWrite memetakan error aturan tulis form ke 422; error lain jatuh ke corehttp.Fail.
func failFormWrite(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrCoreFieldsRequired):
		corehttp.Fail(c, coreerrors.New("LANDING_FORM_CORE_FIELDS", coreFieldsMessage, http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrInvalidDefaultValue),
		errors.Is(err, service.ErrInvalidWhatsAppNumber),
		errors.Is(err, service.ErrInvalidAfterSubmit),
		errors.Is(err, service.ErrInvalidFieldType):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
	default:
		corehttp.Fail(c, err)
	}
}

func afterSubmitFromRequest(v *string) domain.AfterSubmitAction {
	if v == nil {
		return ""
	}
	return domain.AfterSubmitAction(*v)
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
