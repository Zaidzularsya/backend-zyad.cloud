package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
)

// requireManualCreation menolak pembuatan invoice/pelanggan manual lewat HTTP selama saklarnya mati.
// Pelanggan harus berasal dari kontak CRM (supaya WhatsApp dan relasinya lurus); invoice dari Sales
// Order, kontrak, dan aktivasi dibuat lewat service dan tidak melewati middleware ini.
func requireManualCreation(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled {
			c.Next()
			return
		}
		corehttp.Fail(c, coreerrors.New(
			"MANUAL_CREATION_DISABLED",
			"Pembuatan invoice dan pelanggan manual dinonaktifkan sementara. Gunakan kontak CRM dan Sales Order.",
			http.StatusForbidden,
		))
		c.Abort()
	}
}
