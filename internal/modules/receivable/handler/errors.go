package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validUUID(s string) bool { return uuidPattern.MatchString(s) }

func validationFail(c *gin.Context, message string) {
	corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity))
}

func requireScope(c *gin.Context) (coretenant.Scope, bool) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return coretenant.Scope{}, false
	}
	return scope, true
}

// invoiceID memvalidasi :id di batas HTTP; id yang bukan UUID = tidak ditemukan (tidak sampai ke DB).
func invoiceID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !validUUID(id) {
		failReceivable(c, receivable.ErrInvoiceNotFound)
		return "", false
	}
	return id, true
}

func accountID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !validUUID(id) {
		failReceivable(c, receivable.ErrAccountNotFound)
		return "", false
	}
	return id, true
}

// allowed: izin tambahan di dalam handler (mis. izin kanal kirim). Semua izin harus dimiliki.
func allowed(perm permissionmiddleware.CombinedPermissionChecker, c *gin.Context, perms ...string) bool {
	if perm == nil || len(perms) == 0 {
		return true
	}
	ctx := c.Request.Context()
	userID := permissionmiddleware.UserID(c)
	if tc, ok := coretenant.FromContext(ctx); ok {
		if perm.CanOrganization(ctx, userID, tc.OrganizationID(), perms) == nil {
			return true
		}
	}
	return perm.Can(ctx, userID, perms) == nil
}

func forbidden(c *gin.Context) {
	corehttp.Fail(c, coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden))
}

func failReceivable(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrChannelUnavailable):
		corehttp.Fail(c, coreerrors.New("CHANNEL_UNAVAILABLE", err.Error(), http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrInvoiceNotSendable):
		corehttp.Fail(c, coreerrors.New("INVOICE_NOT_SENDABLE", "Invoice dengan status ini tidak bisa dikirim.", http.StatusConflict))
	case errors.Is(err, repository.ErrInvoiceNotDraft):
		corehttp.Fail(c, coreerrors.New("INVOICE_NOT_DRAFT", "Invoice sudah diterbitkan dan tidak bisa diubah.", http.StatusConflict))
	case errors.Is(err, repository.ErrInvoiceNotPayable):
		corehttp.Fail(c, coreerrors.New("INVOICE_NOT_PAYABLE", "Status invoice tidak mengizinkan tindakan ini.", http.StatusConflict))
	case errors.Is(err, repository.ErrInvoiceHasPayments):
		corehttp.Fail(c, coreerrors.New("INVOICE_HAS_PAYMENTS", "Invoice sudah punya pembayaran dan tidak bisa dibatalkan.", http.StatusConflict))
	case errors.Is(err, repository.ErrPaymentExceeds):
		corehttp.Fail(c, coreerrors.New("PAYMENT_EXCEEDS_BALANCE", "Jumlah pembayaran melebihi sisa tagihan.", http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrInvalidPayment):
		validationFail(c, "Jumlah harus lebih dari 0 (maksimal 2 desimal) dan tanggal bayar tidak boleh di masa depan.")
	case errors.Is(err, service.ErrVoidReasonRequired):
		validationFail(c, "Alasan pembatalan wajib diisi (1–500 karakter).")
	case errors.Is(err, service.ErrInvalidInvoice), errors.Is(err, service.ErrInvalidAccount), errors.Is(err, service.ErrInvalidSettings):
		validationFail(c, err.Error())
	case errors.Is(err, receivable.ErrInvoiceNotFound):
		corehttp.Fail(c, coreerrors.New("INVOICE_NOT_FOUND", "Invoice tidak ditemukan.", http.StatusNotFound))
	case errors.Is(err, receivable.ErrAccountNotFound):
		corehttp.Fail(c, coreerrors.New("ACCOUNT_NOT_FOUND", "Pelanggan tidak ditemukan.", http.StatusNotFound))
	case errors.Is(err, service.ErrDocumentsUnavailable):
		corehttp.Fail(c, coreerrors.New("INVOICE_PDF_FAILED", "PDF tidak dapat dibuat.", http.StatusBadGateway))
	default:
		corehttp.Fail(c, err)
	}
}

// failInvoiceDocument: galat render/storage yang bukan sentinel dilaporkan sebagai INVOICE_PDF_FAILED
// (detail hanya di log), sedangkan sentinel bisnis tetap dipetakan seperti biasa.
func failInvoiceDocument(c *gin.Context, err error) {
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) || errors.Is(err, receivable.ErrInvoiceNotFound) || errors.Is(err, service.ErrDocumentsUnavailable) ||
		errors.Is(err, service.ErrInvoiceNotSendable) {
		failReceivable(c, err)
		return
	}
	slog.ErrorContext(c.Request.Context(), "invoice document failed", "invoice_id", c.Param("id"), "error", err)
	corehttp.Fail(c, coreerrors.New("INVOICE_PDF_FAILED", "PDF tidak dapat dibuat.", http.StatusBadGateway))
}

func pageParams(page, perPage int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

func trim(s string) string { return strings.TrimSpace(s) }
