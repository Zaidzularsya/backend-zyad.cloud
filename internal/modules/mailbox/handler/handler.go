package handler

import (
	"errors"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	assetservice "zyad.cloud/internal/modules/asset/service"
	crmdto "zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/dto"
	"zyad.cloud/internal/modules/mailbox/service"
	"zyad.cloud/internal/platform/storage"
	"zyad.cloud/internal/shared/response"
)

// maxFormMemory: attachments above this are buffered to temp files by
// net/http; the service caps the total at 18MB anyway.
const maxFormMemory = 8 << 20

type Handler struct {
	mailboxes *service.MailboxService
	messages  *service.MessageService
}

func NewHandler(mailboxes *service.MailboxService, messages *service.MessageService) *Handler {
	return &Handler{mailboxes: mailboxes, messages: messages}
}

// RegisterRoutes: every route acts on the caller's own mailboxes only, so
// email.read / email.send are enough (there is no "read all").
func (h *Handler) RegisterRoutes(mailboxes, emails *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	read := permissionmiddleware.RequireOrganizationOrGlobal(p, "email.read")
	send := permissionmiddleware.RequireOrganizationOrGlobal(p, "email.send")

	mailboxes.GET("", read, h.ListMailboxes)
	mailboxes.POST("", send, h.CreateMailbox)
	mailboxes.PATCH("/:id", send, h.UpdateMailbox)
	mailboxes.DELETE("/:id", send, h.DeleteMailbox)
	mailboxes.POST("/:id/test", send, h.TestMailbox)
	mailboxes.POST("/:id/messages", send, h.Send)

	emails.GET("", read, h.ListMessages)
	emails.GET("/:id", read, h.GetMessage)
	emails.GET("/:id/attachments/:attachmentId/download", read, h.DownloadAttachment)
}

func requireCaller(c *gin.Context) (coretenant.Scope, string, bool) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	userID := permissionmiddleware.UserID(c)
	if err != nil || userID == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return coretenant.Scope{}, "", false
	}
	return scope, userID, true
}

func (h *Handler) ListMailboxes(c *gin.Context) {
	scope, userID, ok := requireCaller(c)
	if !ok {
		return
	}
	mailboxes, err := h.mailboxes.List(c.Request.Context(), scope, userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.MailboxListFromDomain(mailboxes))
}

func bindMailbox(c *gin.Context) (service.MailboxInput, bool) {
	var req dto.MailboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return service.MailboxInput{}, false
	}
	return service.MailboxInput{
		EmailAddress: req.EmailAddress,
		DisplayName:  req.DisplayName,
		Username:     req.Username,
		Password:     req.Password,
		SMTPHost:     req.SMTPHost,
		SMTPPort:     req.SMTPPort,
		SMTPSecurity: domain.Security(req.SMTPSecurity),
		IMAPHost:     req.IMAPHost,
		IMAPPort:     req.IMAPPort,
		IMAPSecurity: domain.Security(req.IMAPSecurity),
	}, true
}

func (h *Handler) CreateMailbox(c *gin.Context) {
	scope, userID, ok := requireCaller(c)
	if !ok {
		return
	}
	input, ok := bindMailbox(c)
	if !ok {
		return
	}
	mailbox, err := h.mailboxes.Create(c.Request.Context(), scope, userID, input)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "created", dto.MailboxFromDomain(mailbox))
}

func (h *Handler) UpdateMailbox(c *gin.Context) {
	scope, userID, ok := requireCaller(c)
	if !ok {
		return
	}
	input, ok := bindMailbox(c)
	if !ok {
		return
	}
	mailbox, err := h.mailboxes.Update(c.Request.Context(), scope, userID, c.Param("id"), input)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "updated", dto.MailboxFromDomain(mailbox))
}

func (h *Handler) DeleteMailbox(c *gin.Context) {
	scope, userID, ok := requireCaller(c)
	if !ok {
		return
	}
	if err := h.mailboxes.Delete(c.Request.Context(), scope, userID, c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "deleted", nil)
}

func (h *Handler) TestMailbox(c *gin.Context) {
	scope, userID, ok := requireCaller(c)
	if !ok {
		return
	}
	mailbox, err := h.mailboxes.Test(c.Request.Context(), scope, userID, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "connection ok", dto.MailboxFromDomain(mailbox))
}

// Send takes multipart/form-data: to/cc/bcc (repeatable), subject,
// body_html, in_reply_to, related_entity_type, related_entity_id, files
// (repeatable). The Idempotency-Key header makes a retried submit return
// the first message. Responds 202: delivery happens in the background.
func (h *Handler) Send(c *gin.Context) {
	scope, userID, ok := requireCaller(c)
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "multipart form is required", http.StatusBadRequest))
		return
	}
	defer form.RemoveAll()

	files, closeFiles, err := openFiles(form.File["files"])
	defer closeFiles()
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "failed to read uploaded file", http.StatusBadRequest))
		return
	}

	message, err := h.messages.Send(c.Request.Context(), scope, userID, c.Param("id"), service.SendInput{
		ClientRequestID:   strings.TrimSpace(c.GetHeader("Idempotency-Key")),
		To:                form.Value["to"],
		Cc:                form.Value["cc"],
		Bcc:               form.Value["bcc"],
		Subject:           firstValue(form.Value, "subject"),
		BodyHTML:          firstValue(form.Value, "body_html"),
		InReplyTo:         firstValue(form.Value, "in_reply_to"),
		RelatedEntityType: domain.RelatedEntityType(firstValue(form.Value, "related_entity_type")),
		RelatedEntityID:   firstValue(form.Value, "related_entity_id"),
		Files:             files,
	})
	if err != nil {
		failAssetError(c, err)
		return
	}
	response.JSON(c, http.StatusAccepted, "queued", dto.MessageFromDomain(message, true), nil)
}

func openFiles(headers []*multipart.FileHeader) ([]service.FileInput, func(), error) {
	opened := []multipart.File{}
	closeAll := func() {
		for _, file := range opened {
			_ = file.Close()
		}
	}
	files := make([]service.FileInput, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			return nil, closeAll, err
		}
		opened = append(opened, file)
		files = append(files, service.FileInput{Filename: header.Filename, Size: header.Size, Content: file})
	}
	return files, closeAll, nil
}

func firstValue(values map[string][]string, key string) string {
	if list := values[key]; len(list) > 0 {
		return list[0]
	}
	return ""
}

func (h *Handler) ListMessages(c *gin.Context) {
	scope, userID, ok := requireCaller(c)
	if !ok {
		return
	}
	var query dto.MessageListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	page := max(query.Page, 1)
	perPage := query.PerPage
	if perPage <= 0 || perPage > 100 {
		perPage = 20
	}
	messages, total, err := h.messages.List(c.Request.Context(), scope, userID, service.MessageListInput{
		MailboxID:         query.MailboxID,
		Participant:       query.Participant,
		Direction:         domain.Direction(query.Direction),
		RelatedEntityType: domain.RelatedEntityType(query.RelatedEntityType),
		RelatedEntityID:   query.RelatedEntityID,
		Page:              page,
		PerPage:           perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "success", dto.MessageListFromDomain(messages), crmdto.BuildMeta(page, perPage, total))
}

func (h *Handler) GetMessage(c *gin.Context) {
	scope, userID, ok := requireCaller(c)
	if !ok {
		return
	}
	message, err := h.messages.Get(c.Request.Context(), scope, userID, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.MessageFromDomain(message, true))
}

func (h *Handler) DownloadAttachment(c *gin.Context) {
	scope, userID, ok := requireCaller(c)
	if !ok {
		return
	}
	result, err := h.messages.AttachmentURL(c.Request.Context(), scope, userID, c.Param("id"), c.Param("attachmentId"))
	if err != nil {
		failAssetError(c, err)
		return
	}
	corehttp.OK(c, "success", result)
}

// failAssetError maps asset-module errors like the CRM attachment handlers.
func failAssetError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, assetservice.ErrQuotaExceeded):
		corehttp.Fail(c, coreerrors.New("STORAGE_QUOTA_EXCEEDED", "kuota storage tenant sudah penuh", http.StatusConflict))
	case errors.Is(err, assetservice.ErrFileTooLarge):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "ukuran file maksimal 10MB per lampiran", http.StatusBadRequest))
	case errors.Is(err, assetservice.ErrInvalidMimeType):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "tipe file lampiran tidak didukung", http.StatusBadRequest))
	case errors.Is(err, assetservice.ErrStorageRequired):
		corehttp.Fail(c, coreerrors.New("STORAGE_NOT_CONFIGURED", err.Error(), http.StatusServiceUnavailable))
	case errors.Is(err, storage.ErrPresignUnsupported):
		corehttp.Fail(c, coreerrors.New("STORAGE_PRESIGN_UNSUPPORTED", "storage provider tidak mendukung presigned url", http.StatusConflict))
	default:
		corehttp.Fail(c, err)
	}
}
