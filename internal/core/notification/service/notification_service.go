package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/notification/dispatcher"
	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/repository"
	notificationtemplate "zyad.cloud/internal/core/notification/template"
)

const (
	defaultNotificationLocale      = "id-ID"
	defaultNotificationMaxAttempts = 3
)

type NotificationTemplateRepository interface {
	FindActiveByCodeChannelLocale(ctx context.Context, code string, channel domain.Channel, locale string) (domain.NotificationTemplate, error)
}

type NotificationLogRepository interface {
	CreatePending(ctx context.Context, log *domain.NotificationLog) error
	FindByID(ctx context.Context, id string) (domain.NotificationLog, error)
	List(ctx context.Context, filter repository.NotificationLogListFilter) ([]domain.NotificationLog, error)
	FindRetryable(ctx context.Context, limit int) ([]domain.NotificationLog, error)
	MarkProcessing(ctx context.Context, id string) error
	MarkSent(ctx context.Context, id string, provider string, providerMessageID string, providerResponse map[string]any) error
	MarkFailed(ctx context.Context, id string, errorMessage string, nextRetryAt *time.Time) error
	MarkDead(ctx context.Context, id string, errorMessage string) error
	MarkCancelled(ctx context.Context, id string, errorMessage string) error
}

type NotificationPreferenceChecker interface {
	IsEnabled(ctx context.Context, userID string, organizationID string, eventType string, channel domain.Channel) (bool, error)
}

type DirectNotificationRequest struct {
	EventID        string
	EventType      string
	OrganizationID string
	Channel        domain.Channel
	Recipient      domain.NotificationRecipient
	Subject        string
	Body           string
	MaxAttempts    int
}

type NotificationService struct {
	templateRepo NotificationTemplateRepository
	logRepo      NotificationLogRepository
	preferences  NotificationPreferenceChecker
	renderer     *notificationtemplate.Renderer
	dispatchers  map[domain.Channel]dispatcher.Dispatcher
	now          func() time.Time
}

func NewNotificationService(
	templateRepo NotificationTemplateRepository,
	logRepo NotificationLogRepository,
	preferences NotificationPreferenceChecker,
	renderer *notificationtemplate.Renderer,
	dispatchers ...dispatcher.Dispatcher,
) *NotificationService {
	if renderer == nil {
		renderer = notificationtemplate.NewRenderer(nil)
	}

	service := &NotificationService{
		templateRepo: templateRepo,
		logRepo:      logRepo,
		preferences:  preferences,
		renderer:     renderer,
		dispatchers:  make(map[domain.Channel]dispatcher.Dispatcher),
		now:          time.Now,
	}
	for _, sender := range dispatchers {
		service.RegisterDispatcher(sender)
	}
	return service
}

func (s *NotificationService) RegisterDispatcher(sender dispatcher.Dispatcher) {
	if sender == nil {
		return
	}
	s.dispatchers[sender.Channel()] = sender
}

func (s *NotificationService) SendByTemplate(ctx context.Context, notification domain.Notification) (domain.NotificationLog, error) {
	if s.templateRepo == nil {
		return domain.NotificationLog{}, coreerrors.New("NOTIFICATION_TEMPLATE_REPOSITORY_REQUIRED", "notification template repository is required", http.StatusInternalServerError)
	}

	notification.Locale = defaultNotificationLocaleIfEmpty(notification.Locale)
	if err := validateNotification(notification); err != nil {
		return domain.NotificationLog{}, err
	}

	template, err := s.templateRepo.FindActiveByCodeChannelLocale(ctx, notification.TemplateCode, notification.Channel, notification.Locale)
	if err != nil {
		return domain.NotificationLog{}, mapNotificationError("NOTIFICATION_TEMPLATE_NOT_FOUND", "active notification template not found", err)
	}
	if !template.CanBeUsed() {
		return domain.NotificationLog{}, coreerrors.New("NOTIFICATION_TEMPLATE_NOT_USABLE", "notification template is not active", http.StatusConflict)
	}

	rendered, err := s.renderer.Render(template, notification.Payload)
	if err != nil {
		return domain.NotificationLog{}, mapNotificationError("NOTIFICATION_RENDER_FAILED", "failed to render notification template", err)
	}

	log := buildTemplateLog(notification, template, rendered)
	return s.createAndDispatch(ctx, &log)
}

func (s *NotificationService) SendDirect(ctx context.Context, req DirectNotificationRequest) (domain.NotificationLog, error) {
	if err := validateDirectNotification(req); err != nil {
		return domain.NotificationLog{}, err
	}

	log := domain.NotificationLog{
		EventID:                req.EventID,
		EventType:              req.EventType,
		OrganizationID:         req.OrganizationID,
		Channel:                req.Channel,
		RecipientType:          defaultRecipientType(req.Recipient.Type),
		RecipientUserID:        req.Recipient.UserID,
		RecipientNameSnapshot:  req.Recipient.Name,
		RecipientEmailSnapshot: req.Recipient.Email,
		RecipientPhoneSnapshot: req.Recipient.Phone,
		Destination:            destinationFor(req.Channel, req.Recipient),
		Subject:                req.Subject,
		Body:                   req.Body,
		Status:                 domain.LogStatusPending,
		MaxAttempts:            maxAttemptsOrDefault(req.MaxAttempts),
	}

	return s.createAndDispatch(ctx, &log)
}

func (s *NotificationService) Retry(ctx context.Context, logID string) (domain.NotificationLog, error) {
	if strings.TrimSpace(logID) == "" {
		return domain.NotificationLog{}, coreerrors.New("NOTIFICATION_LOG_ID_REQUIRED", "notification log id is required", http.StatusBadRequest)
	}

	log, err := s.findLog(ctx, logID)
	if err != nil {
		return domain.NotificationLog{}, err
	}
	if log.Status != domain.LogStatusFailed && log.Status != domain.LogStatusDead {
		return domain.NotificationLog{}, coreerrors.New("NOTIFICATION_RETRY_NOT_ALLOWED", "notification log cannot be retried", http.StatusConflict)
	}

	return s.dispatchLog(ctx, log)
}

func (s *NotificationService) GetLog(ctx context.Context, logID string) (domain.NotificationLog, error) {
	if strings.TrimSpace(logID) == "" {
		return domain.NotificationLog{}, coreerrors.New("NOTIFICATION_LOG_ID_REQUIRED", "notification log id is required", http.StatusBadRequest)
	}
	return s.findLog(ctx, logID)
}

func (s *NotificationService) ListLogs(ctx context.Context, filter repository.NotificationLogListFilter) ([]domain.NotificationLog, error) {
	if s.logRepo == nil {
		return nil, coreerrors.New("NOTIFICATION_LOG_REPOSITORY_REQUIRED", "notification log repository is required", http.StatusInternalServerError)
	}
	if filter.Channel != "" && !filter.Channel.IsValid() {
		return nil, coreerrors.New("NOTIFICATION_CHANNEL_INVALID", "notification channel is invalid", http.StatusBadRequest)
	}
	if filter.Status != "" && !filter.Status.IsValid() {
		return nil, coreerrors.New("NOTIFICATION_STATUS_INVALID", "notification log status is invalid", http.StatusBadRequest)
	}

	logs, err := s.logRepo.List(ctx, filter)
	if err != nil {
		return nil, mapNotificationError("NOTIFICATION_LOG_LIST_FAILED", "failed to list notification logs", err)
	}
	return logs, nil
}

func (s *NotificationService) RetryDue(ctx context.Context, limit int) ([]domain.NotificationLog, error) {
	if s.logRepo == nil {
		return nil, coreerrors.New("NOTIFICATION_LOG_REPOSITORY_REQUIRED", "notification log repository is required", http.StatusInternalServerError)
	}

	logs, err := s.logRepo.FindRetryable(ctx, limit)
	if err != nil {
		return nil, mapNotificationError("NOTIFICATION_RETRY_LIST_FAILED", "failed to list retryable notifications", err)
	}

	results := make([]domain.NotificationLog, 0, len(logs))
	for _, log := range logs {
		result, err := s.dispatchLog(ctx, log)
		if err != nil {
			results = append(results, result)
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *NotificationService) Cancel(ctx context.Context, logID string) (domain.NotificationLog, error) {
	if strings.TrimSpace(logID) == "" {
		return domain.NotificationLog{}, coreerrors.New("NOTIFICATION_LOG_ID_REQUIRED", "notification log id is required", http.StatusBadRequest)
	}

	log, err := s.findLog(ctx, logID)
	if err != nil {
		return domain.NotificationLog{}, err
	}
	if log.Status != domain.LogStatusPending {
		return domain.NotificationLog{}, coreerrors.New("NOTIFICATION_CANCEL_NOT_ALLOWED", "only pending notification can be cancelled", http.StatusConflict)
	}
	if err := s.logRepo.MarkCancelled(ctx, log.ID, "cancelled by request"); err != nil {
		return domain.NotificationLog{}, mapNotificationError("NOTIFICATION_CANCEL_FAILED", "failed to cancel notification", err)
	}

	return s.findLog(ctx, log.ID)
}

func (s *NotificationService) createAndDispatch(ctx context.Context, log *domain.NotificationLog) (domain.NotificationLog, error) {
	if s.logRepo == nil {
		return domain.NotificationLog{}, coreerrors.New("NOTIFICATION_LOG_REPOSITORY_REQUIRED", "notification log repository is required", http.StatusInternalServerError)
	}
	if log.Status == "" {
		log.Status = domain.LogStatusPending
	}
	if log.MaxAttempts == 0 {
		log.MaxAttempts = defaultNotificationMaxAttempts
	}

	if err := s.logRepo.CreatePending(ctx, log); err != nil {
		return domain.NotificationLog{}, mapNotificationError("NOTIFICATION_LOG_CREATE_FAILED", "failed to create notification log", err)
	}

	enabled, err := s.preferenceEnabled(ctx, *log)
	if err != nil {
		return *log, err
	}
	if !enabled {
		if err := s.logRepo.MarkCancelled(ctx, log.ID, "notification disabled by user preference"); err != nil {
			return *log, mapNotificationError("NOTIFICATION_CANCEL_FAILED", "failed to cancel notification", err)
		}
		return s.findLog(ctx, log.ID)
	}

	return s.dispatchLog(ctx, *log)
}

func (s *NotificationService) dispatchLog(ctx context.Context, log domain.NotificationLog) (domain.NotificationLog, error) {
	sender, ok := s.dispatchers[log.Channel]
	if !ok {
		err := coreerrors.New("NOTIFICATION_DISPATCHER_NOT_FOUND", "notification dispatcher is not registered", http.StatusConflict)
		return s.markDispatchFailed(ctx, log, err)
	}

	if err := s.logRepo.MarkProcessing(ctx, log.ID); err != nil {
		return domain.NotificationLog{}, mapNotificationError("NOTIFICATION_MARK_PROCESSING_FAILED", "failed to mark notification as processing", err)
	}

	result, err := sender.Send(ctx, dispatcher.Message{
		Channel:     log.Channel,
		Destination: log.Destination,
		Subject:     log.Subject,
		Body:        log.Body,
		Recipient: dispatcher.Recipient{
			Type:   log.RecipientType,
			UserID: log.RecipientUserID,
			Name:   log.RecipientNameSnapshot,
			Email:  log.RecipientEmailSnapshot,
			Phone:  log.RecipientPhoneSnapshot,
		},
		Metadata: map[string]any{
			"log_id":        log.ID,
			"event_id":      log.EventID,
			"event_type":    log.EventType,
			"template_code": log.TemplateCode,
		},
	})
	if err != nil {
		return s.markDispatchFailed(ctx, log, err)
	}

	if err := s.logRepo.MarkSent(ctx, log.ID, result.Provider, result.ProviderMessageID, result.RawResponse); err != nil {
		return domain.NotificationLog{}, mapNotificationError("NOTIFICATION_MARK_SENT_FAILED", "failed to mark notification as sent", err)
	}

	return s.findLog(ctx, log.ID)
}

func (s *NotificationService) markDispatchFailed(ctx context.Context, log domain.NotificationLog, cause error) (domain.NotificationLog, error) {
	errorMessage := cause.Error()
	if log.Attempts+1 >= log.MaxAttempts {
		if err := s.logRepo.MarkDead(ctx, log.ID, errorMessage); err != nil {
			return domain.NotificationLog{}, mapNotificationError("NOTIFICATION_MARK_DEAD_FAILED", "failed to mark notification as dead", err)
		}
		updated, _ := s.findLog(ctx, log.ID)
		return updated, mapNotificationError("NOTIFICATION_DISPATCH_FAILED", "failed to dispatch notification", cause)
	}

	nextRetryAt := s.now().UTC().Add(time.Minute)
	if err := s.logRepo.MarkFailed(ctx, log.ID, errorMessage, &nextRetryAt); err != nil {
		return domain.NotificationLog{}, mapNotificationError("NOTIFICATION_MARK_FAILED_FAILED", "failed to mark notification as failed", err)
	}
	updated, _ := s.findLog(ctx, log.ID)
	return updated, mapNotificationError("NOTIFICATION_DISPATCH_FAILED", "failed to dispatch notification", cause)
}

func (s *NotificationService) preferenceEnabled(ctx context.Context, log domain.NotificationLog) (bool, error) {
	if s.preferences == nil || log.RecipientUserID == "" || log.EventType == "" {
		return true, nil
	}

	enabled, err := s.preferences.IsEnabled(ctx, log.RecipientUserID, log.OrganizationID, log.EventType, log.Channel)
	if err != nil {
		return false, mapNotificationError("NOTIFICATION_PREFERENCE_CHECK_FAILED", "failed to check notification preference", err)
	}
	return enabled, nil
}

func (s *NotificationService) findLog(ctx context.Context, id string) (domain.NotificationLog, error) {
	if s.logRepo == nil {
		return domain.NotificationLog{}, coreerrors.New("NOTIFICATION_LOG_REPOSITORY_REQUIRED", "notification log repository is required", http.StatusInternalServerError)
	}

	log, err := s.logRepo.FindByID(ctx, id)
	if err != nil {
		return domain.NotificationLog{}, mapNotificationError("NOTIFICATION_LOG_NOT_FOUND", "notification log not found", err)
	}
	return log, nil
}

func buildTemplateLog(notification domain.Notification, template domain.NotificationTemplate, rendered notificationtemplate.RenderedTemplate) domain.NotificationLog {
	recipientUserID := firstNonEmpty(notification.Recipient.UserID, notification.UserID)
	return domain.NotificationLog{
		EventID:                notification.EventID,
		EventType:              notification.EventType,
		TemplateID:             template.ID,
		TemplateCode:           template.Code,
		TemplateVersion:        template.Version,
		OrganizationID:         notification.OrganizationID,
		Channel:                notification.Channel,
		RecipientType:          defaultRecipientType(notification.Recipient.Type),
		RecipientUserID:        recipientUserID,
		RecipientNameSnapshot:  notification.Recipient.Name,
		RecipientEmailSnapshot: notification.Recipient.Email,
		RecipientPhoneSnapshot: notification.Recipient.Phone,
		Destination:            destinationFor(notification.Channel, notification.Recipient),
		Subject:                rendered.Subject,
		Body:                   rendered.Body,
		Status:                 domain.LogStatusPending,
		MaxAttempts:            defaultNotificationMaxAttempts,
	}
}

func validateNotification(notification domain.Notification) error {
	if strings.TrimSpace(notification.TemplateCode) == "" {
		return coreerrors.New("NOTIFICATION_TEMPLATE_CODE_REQUIRED", "notification template code is required", http.StatusBadRequest)
	}
	if !notification.Channel.IsValid() {
		return coreerrors.New("NOTIFICATION_CHANNEL_INVALID", "notification channel is invalid", http.StatusBadRequest)
	}
	if strings.TrimSpace(destinationFor(notification.Channel, notification.Recipient)) == "" {
		return coreerrors.New("NOTIFICATION_DESTINATION_REQUIRED", "notification destination is required", http.StatusBadRequest)
	}
	return nil
}

func validateDirectNotification(req DirectNotificationRequest) error {
	if !req.Channel.IsValid() {
		return coreerrors.New("NOTIFICATION_CHANNEL_INVALID", "notification channel is invalid", http.StatusBadRequest)
	}
	if strings.TrimSpace(destinationFor(req.Channel, req.Recipient)) == "" {
		return coreerrors.New("NOTIFICATION_DESTINATION_REQUIRED", "notification destination is required", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Body) == "" {
		return coreerrors.New("NOTIFICATION_BODY_REQUIRED", "notification body is required", http.StatusBadRequest)
	}
	if req.Channel == domain.ChannelEmail && strings.TrimSpace(req.Subject) == "" {
		return coreerrors.New("NOTIFICATION_SUBJECT_REQUIRED", "email notification subject is required", http.StatusBadRequest)
	}
	return nil
}

func destinationFor(channel domain.Channel, recipient domain.NotificationRecipient) string {
	if strings.TrimSpace(recipient.Destination) != "" {
		return recipient.Destination
	}
	switch channel {
	case domain.ChannelEmail:
		return recipient.Email
	case domain.ChannelWhatsApp:
		return recipient.Phone
	case domain.ChannelInApp:
		return firstNonEmpty(recipient.UserID, recipient.Email, recipient.Phone)
	default:
		return recipient.Destination
	}
}

func defaultRecipientType(recipientType string) string {
	if strings.TrimSpace(recipientType) == "" {
		return "user"
	}
	return recipientType
}

func defaultNotificationLocaleIfEmpty(locale string) string {
	if strings.TrimSpace(locale) == "" {
		return defaultNotificationLocale
	}
	return locale
}

func maxAttemptsOrDefault(maxAttempts int) int {
	if maxAttempts <= 0 {
		return defaultNotificationMaxAttempts
	}
	return maxAttempts
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mapNotificationError(code string, message string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New(code, message, http.StatusNotFound)
	}
	if errors.Is(err, notificationtemplate.ErrMalformedVariable) ||
		errors.Is(err, notificationtemplate.ErrUnknownVariable) ||
		errors.Is(err, notificationtemplate.ErrMissingRequiredVariable) {
		return coreerrors.Wrap(code, err.Error(), http.StatusUnprocessableEntity, err)
	}

	var validationErr notificationtemplate.ValidationError
	if errors.As(err, &validationErr) {
		return coreerrors.Wrap(code, fmt.Sprintf("%s: %s", message, validationErr.Error()), http.StatusUnprocessableEntity, err)
	}

	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) {
		return err
	}

	return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
}

var _ NotificationTemplateRepository = (*repository.TemplateRepository)(nil)
var _ NotificationLogRepository = (*repository.NotificationLogRepository)(nil)
var _ NotificationPreferenceChecker = (*repository.PreferenceRepository)(nil)
