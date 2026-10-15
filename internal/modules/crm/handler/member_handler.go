package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/service"
)

type MemberHandler struct {
	svc service.MemberService
}

func NewMemberHandler(svc service.MemberService) *MemberHandler {
	return &MemberHandler{svc: svc}
}

// RegisterRoutes mendaftarkan lookup anggota organization untuk dropdown
// owner lead. Di-gate lead.read (bukan user.read) karena hanya mengembalikan
// id/nama/email anggota aktif organization yang sama — endpoint user
// management yang lengkap tetap butuh user.read.
func (h *MemberHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/members", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.List)
}

func (h *MemberHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	members, err := h.svc.ListActive(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.MemberListFromDomain(members))
}
