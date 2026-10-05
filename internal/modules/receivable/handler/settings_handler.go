package handler

import (
	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/dto"
	"zyad.cloud/internal/modules/receivable/service"
)

type SettingsHandler struct {
	svc  service.SettingsService
	perm permissionmiddleware.CombinedPermissionChecker
}

func NewSettingsHandler(svc service.SettingsService) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

func (h *SettingsHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	h.perm = p
	// GET dibaca form invoice (kanal default), jadi cukup invoice.read; mengubah butuh receivable.settings.
	router.GET("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.Get)
	router.PUT("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "receivable.settings"), h.Update)
	router.GET("/members/senders", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.Senders)
}

func (h *SettingsHandler) Get(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	s, err := h.svc.Get(c.Request.Context(), scope)
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SettingsFromDomain(s))
}

func (h *SettingsHandler) Update(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var req dto.SettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	sender := trim(req.DefaultSenderUserID)
	if sender != "" && !validUUID(sender) {
		validationFail(c, "Pengirim default tidak valid.")
		return
	}
	s, err := h.svc.Update(c.Request.Context(), scope, domain.Settings{
		InvoiceLeadDays: req.InvoiceLeadDays, PaymentTermsDays: req.PaymentTermsDays,
		DefaultChannels: req.DefaultChannels, DefaultSenderUserID: sender,
	}, permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SettingsFromDomain(s))
}

// Senders: pilihan pengirim default dan PIC. Pengelola pengaturan dan pembuat invoice sama-sama butuh daftar ini.
func (h *SettingsHandler) Senders(c *gin.Context) {
	if !allowed(h.perm, c, "receivable.settings") && !allowed(h.perm, c, "invoice.create") {
		forbidden(c)
		return
	}
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	members, err := h.svc.Senders(c.Request.Context(), scope)
	if err != nil {
		failReceivable(c, err)
		return
	}
	out := make([]dto.SenderResponse, 0, len(members))
	for _, m := range members {
		out = append(out, dto.SenderResponse{UserID: m.UserID, Name: m.Name, Email: m.Email})
	}
	corehttp.OK(c, "success", out)
}
