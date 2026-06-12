package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/dto"
	"zyad.cloud/internal/core/notification/repository"
	notificationtemplate "zyad.cloud/internal/core/notification/template"
)

type TemplateRepository interface {
	Create(ctx context.Context, template *domain.NotificationTemplate) error
	FindByID(ctx context.Context, id string) (domain.NotificationTemplate, error)
	List(ctx context.Context, filter repository.TemplateListFilter) ([]domain.NotificationTemplate, error)
	Update(ctx context.Context, template *domain.NotificationTemplate) error
	SoftDelete(ctx context.Context, id string) error
	Activate(ctx context.Context, id string) error
	Deactivate(ctx context.Context, id string) error
	Archive(ctx context.Context, id string) error
}

type TemplateService struct {
	repo      TemplateRepository
	validator *notificationtemplate.Validator
	renderer  *notificationtemplate.Renderer
}

func NewTemplateService(repo TemplateRepository, validator *notificationtemplate.Validator, renderer *notificationtemplate.Renderer) *TemplateService {
	if validator == nil {
		validator = notificationtemplate.NewValidator(nil)
	}
	if renderer == nil {
		renderer = notificationtemplate.NewRenderer(nil)
	}
	return &TemplateService{
		repo:      repo,
		validator: validator,
		renderer:  renderer,
	}
}

func (s *TemplateService) CreateTemplate(ctx context.Context, req dto.CreateTemplateRequest) (domain.NotificationTemplate, error) {
	template := domain.NotificationTemplate{
		Code:               req.Code,
		Name:               req.Name,
		Description:        req.Description,
		Channel:            domain.Channel(req.Channel),
		Locale:             defaultLocale(req.Locale),
		SubjectTemplate:    req.SubjectTemplate,
		BodyTemplate:       req.BodyTemplate,
		AvailableVariables: variableItemsToDomain(req.AvailableVariables),
		SamplePayload:      req.SamplePayload,
		Status:             domain.TemplateStatusDraft,
		IsSystem:           req.IsSystem,
		IsActive:           req.IsActive,
		Version:            req.Version,
	}
	if template.Version == 0 {
		template.Version = 1
	}

	if err := s.validator.Validate(template); err != nil {
		return domain.NotificationTemplate{}, mapTemplateError("TEMPLATE_VALIDATION_FAILED", "template validation failed", err)
	}
	if err := s.repo.Create(ctx, &template); err != nil {
		return domain.NotificationTemplate{}, mapTemplateError("TEMPLATE_CREATE_FAILED", "failed to create notification template", err)
	}

	return template, nil
}

func (s *TemplateService) UpdateTemplate(ctx context.Context, id string, req dto.UpdateTemplateRequest) (domain.NotificationTemplate, error) {
	template, err := s.GetTemplate(ctx, id)
	if err != nil {
		return domain.NotificationTemplate{}, err
	}

	if req.Name != nil {
		template.Name = *req.Name
	}
	if req.Description != nil {
		template.Description = *req.Description
	}
	if req.Locale != nil {
		template.Locale = *req.Locale
	}
	if req.SubjectTemplate != nil {
		template.SubjectTemplate = *req.SubjectTemplate
	}
	if req.BodyTemplate != nil {
		template.BodyTemplate = *req.BodyTemplate
	}
	if req.AvailableVariables != nil {
		template.AvailableVariables = variableItemsToDomain(req.AvailableVariables)
	}
	if req.SamplePayload != nil {
		template.SamplePayload = req.SamplePayload
	}
	if req.IsActive != nil {
		template.IsActive = *req.IsActive
	}

	if err := s.validator.Validate(template); err != nil {
		return domain.NotificationTemplate{}, mapTemplateError("TEMPLATE_VALIDATION_FAILED", "template validation failed", err)
	}
	if err := s.repo.Update(ctx, &template); err != nil {
		return domain.NotificationTemplate{}, mapTemplateError("TEMPLATE_UPDATE_FAILED", "failed to update notification template", err)
	}

	return template, nil
}

func (s *TemplateService) GetTemplate(ctx context.Context, id string) (domain.NotificationTemplate, error) {
	template, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return domain.NotificationTemplate{}, mapTemplateError("TEMPLATE_NOT_FOUND", "notification template not found", err)
	}
	if template.IsDeleted() {
		return domain.NotificationTemplate{}, coreerrors.New("TEMPLATE_NOT_FOUND", "notification template not found", http.StatusNotFound)
	}
	return template, nil
}

func (s *TemplateService) ListTemplates(ctx context.Context, filter repository.TemplateListFilter) ([]domain.NotificationTemplate, error) {
	templates, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, mapTemplateError("TEMPLATE_LIST_FAILED", "failed to list notification templates", err)
	}
	return templates, nil
}

func (s *TemplateService) DeleteTemplate(ctx context.Context, id string) error {
	template, err := s.GetTemplate(ctx, id)
	if err != nil {
		return err
	}
	if template.IsSystem {
		return coreerrors.New("TEMPLATE_SYSTEM_DELETE_FORBIDDEN", "system template cannot be deleted", http.StatusForbidden)
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return mapTemplateError("TEMPLATE_DELETE_FAILED", "failed to delete notification template", err)
	}
	return nil
}

func (s *TemplateService) PreviewTemplate(ctx context.Context, id string, payload map[string]any) (notificationtemplate.RenderedTemplate, error) {
	template, err := s.GetTemplate(ctx, id)
	if err != nil {
		return notificationtemplate.RenderedTemplate{}, err
	}

	rendered, err := s.renderer.Preview(template, payload)
	if err != nil {
		return notificationtemplate.RenderedTemplate{}, mapTemplateError("TEMPLATE_PREVIEW_FAILED", "failed to preview notification template", err)
	}
	return rendered, nil
}

func (s *TemplateService) ActivateTemplate(ctx context.Context, id string) error {
	template, err := s.GetTemplate(ctx, id)
	if err != nil {
		return err
	}
	if err := s.validator.Validate(template); err != nil {
		return mapTemplateError("TEMPLATE_VALIDATION_FAILED", "template validation failed", err)
	}
	if err := s.repo.Activate(ctx, id); err != nil {
		return mapTemplateError("TEMPLATE_ACTIVATE_FAILED", "failed to activate notification template", err)
	}
	return nil
}

func (s *TemplateService) DeactivateTemplate(ctx context.Context, id string) error {
	if _, err := s.GetTemplate(ctx, id); err != nil {
		return err
	}
	if err := s.repo.Deactivate(ctx, id); err != nil {
		return mapTemplateError("TEMPLATE_DEACTIVATE_FAILED", "failed to deactivate notification template", err)
	}
	return nil
}

func (s *TemplateService) ArchiveTemplate(ctx context.Context, id string) error {
	if _, err := s.GetTemplate(ctx, id); err != nil {
		return err
	}
	if err := s.repo.Archive(ctx, id); err != nil {
		return mapTemplateError("TEMPLATE_ARCHIVE_FAILED", "failed to archive notification template", err)
	}
	return nil
}

func (s *TemplateService) CloneTemplate(ctx context.Context, id string) (domain.NotificationTemplate, error) {
	source, err := s.GetTemplate(ctx, id)
	if err != nil {
		return domain.NotificationTemplate{}, err
	}

	latest, err := s.repo.List(ctx, repository.TemplateListFilter{
		Code:    source.Code,
		Channel: source.Channel,
		Locale:  source.Locale,
		Limit:   1,
	})
	if err != nil {
		return domain.NotificationTemplate{}, mapTemplateError("TEMPLATE_CLONE_FAILED", "failed to clone notification template", err)
	}

	nextVersion := source.Version + 1
	if len(latest) > 0 && latest[0].Version >= nextVersion {
		nextVersion = latest[0].Version + 1
	}

	clone := source
	clone.ID = ""
	clone.Status = domain.TemplateStatusDraft
	clone.IsActive = false
	clone.IsSystem = false
	clone.Version = nextVersion
	clone.CreatedBy = ""
	clone.UpdatedBy = ""
	clone.CreatedAt = source.CreatedAt
	clone.UpdatedAt = source.UpdatedAt
	clone.DeletedAt = nil

	if err := s.validator.Validate(clone); err != nil {
		return domain.NotificationTemplate{}, mapTemplateError("TEMPLATE_VALIDATION_FAILED", "template validation failed", err)
	}
	if err := s.repo.Create(ctx, &clone); err != nil {
		return domain.NotificationTemplate{}, mapTemplateError("TEMPLATE_CLONE_FAILED", "failed to clone notification template", err)
	}

	return clone, nil
}

func variableItemsToDomain(items []dto.VariableItem) []domain.NotificationVariable {
	variables := make([]domain.NotificationVariable, 0, len(items))
	for _, item := range items {
		variables = append(variables, domain.NotificationVariable{
			Key:         item.Key,
			Description: item.Description,
			Required:    item.Required,
			Example:     item.Example,
		})
	}
	return variables
}

func defaultLocale(locale string) string {
	if locale == "" {
		return "id-ID"
	}
	return locale
}

func mapTemplateError(code string, message string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New("TEMPLATE_NOT_FOUND", "notification template not found", http.StatusNotFound)
	}

	var validationErr notificationtemplate.ValidationError
	if errors.As(err, &validationErr) {
		return coreerrors.Wrap(code, formatValidationMessage(message, validationErr), http.StatusUnprocessableEntity, err)
	}
	if errors.Is(err, notificationtemplate.ErrMalformedVariable) ||
		errors.Is(err, notificationtemplate.ErrUnknownVariable) ||
		errors.Is(err, notificationtemplate.ErrMissingRequiredVariable) {
		return coreerrors.Wrap(code, err.Error(), http.StatusUnprocessableEntity, err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return coreerrors.Wrap("TEMPLATE_ALREADY_EXISTS", "notification template already exists", http.StatusConflict, err)
	}

	return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
}

func formatValidationMessage(fallback string, err notificationtemplate.ValidationError) string {
	if len(err.Fields) == 0 {
		return fallback
	}
	return fmt.Sprintf("%s: %s", fallback, err.Fields[0].Message)
}
