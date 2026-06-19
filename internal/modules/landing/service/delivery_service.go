package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var (
	ErrInvalidIntegrationType = errors.New("invalid integration type")
)

type deliveryService struct {
	integrationRepo repository.IntegrationRepository
	submissionRepo  repository.SubmissionRepository
	httpClient      *http.Client
}

func NewDeliveryService(
	integrationRepo repository.IntegrationRepository,
	submissionRepo repository.SubmissionRepository,
) DeliveryService {
	return &deliveryService{
		integrationRepo: integrationRepo,
		submissionRepo:  submissionRepo,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Integrations

func (s *deliveryService) CreateIntegration(ctx context.Context, scope coretenant.Scope, params repository.CreateIntegrationParams) (domain.LandingLeadIntegration, error) {
	if !scope.IsValid() {
		return domain.LandingLeadIntegration{}, coretenant.ErrInvalidScope
	}

	if !params.Type.IsValid() {
		return domain.LandingLeadIntegration{}, ErrInvalidIntegrationType
	}

	return s.integrationRepo.CreateIntegration(ctx, scope, params)
}

func (s *deliveryService) FindIntegrationByID(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingLeadIntegration, error) {
	if !scope.IsValid() {
		return domain.LandingLeadIntegration{}, coretenant.ErrInvalidScope
	}
	return s.integrationRepo.GetIntegration(ctx, scope, id)
}

func (s *deliveryService) ListIntegrations(ctx context.Context, scope coretenant.Scope) ([]domain.LandingLeadIntegration, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}
	return s.integrationRepo.ListIntegrations(ctx, scope)
}

func (s *deliveryService) UpdateIntegration(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateIntegrationParams) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	return s.integrationRepo.UpdateIntegration(ctx, scope, id, params)
}

func (s *deliveryService) DeleteIntegration(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	return s.integrationRepo.DeleteIntegration(ctx, scope, id)
}

// Dispatch

func (s *deliveryService) DispatchFormSubmission(ctx context.Context, scope coretenant.Scope, submissionID string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	// 1. Check if submission exists
	_, err := s.submissionRepo.FindByID(ctx, scope, submissionID)
	if err != nil {
		return err
	}

	// 2. Get active integrations
	integrations, err := s.integrationRepo.ListIntegrations(ctx, scope)
	if err != nil {
		return err
	}

	// 3. Create delivery log for each active integration
	for _, integration := range integrations {
		if !integration.IsActive {
			continue
		}

		// (Optional) Here we could check integration.EventFilters against the submission
		// to see if it actually matches. For now, dispatch to all active.

		logParams := repository.CreateDeliveryLogParams{
			IntegrationID: integration.ID,
			SubmissionID:  submissionID,
			Status:        domain.DeliveryStatusPending,
		}

		_, err := s.integrationRepo.CreateDeliveryLog(ctx, scope, logParams)
		if err != nil {
			// In production, might log error and continue, but we return for strictness
			return err
		}
	}

	return nil
}

// Worker Operations

func (s *deliveryService) ProcessPendingDeliveries(ctx context.Context, scope coretenant.Scope, limit int) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	logs, err := s.integrationRepo.ClaimPendingDeliveries(ctx, scope, limit)
	if err != nil {
		return err
	}

	for _, log := range logs {
		s.processDelivery(ctx, log)
	}

	return nil
}

func (s *deliveryService) processDelivery(ctx context.Context, log domain.LandingLeadDeliveryLog) {
	tc, _ := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     log.OrganizationID,
		OrganizationSlug:   "worker-process",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceWorker,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	scope, _ := coretenant.NewScope(tc)

	integration, err := s.integrationRepo.GetIntegration(ctx, scope, log.IntegrationID)
	if err != nil {
		s.markFailed(ctx, log, fmt.Errorf("failed to get integration: %w", err))
		return
	}

	submission, err := s.submissionRepo.FindByID(ctx, scope, log.SubmissionID)
	if err != nil {
		s.markFailed(ctx, log, fmt.Errorf("failed to get submission: %w", err))
		return
	}

	var dispatchErr error
	var responsePayload string

	switch integration.Type {
	case domain.IntegrationTypeWebhook:
		responsePayload, dispatchErr = s.dispatchWebhook(ctx, integration, submission)
	default:
		dispatchErr = fmt.Errorf("unsupported integration type: %s", integration.Type)
	}

	if dispatchErr != nil {
		s.markFailed(ctx, log, dispatchErr)
		return
	}

	// Success
	s.integrationRepo.UpdateDeliveryLogStatus(ctx, scope, log.ID, repository.UpdateDeliveryLogParams{
		Status:          domain.DeliveryStatusSuccess,
		ResponsePayload: &responsePayload,
	})
}

func (s *deliveryService) dispatchWebhook(ctx context.Context, integration domain.LandingLeadIntegration, submission domain.LandingSubmission) (string, error) {
	urlAny, ok := integration.Credentials["url"]
	if !ok {
		return "", errors.New("webhook url is missing in credentials")
	}
	url, ok := urlAny.(string)
	if !ok || url == "" {
		return "", errors.New("webhook url is invalid")
	}

	payloadBytes, err := json.Marshal(submission)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	// Apply HMAC signature if secret is provided
	if secretAny, ok := integration.Credentials["secret"]; ok {
		if secretStr, ok := secretAny.(string); ok && secretStr != "" {
			mac := hmac.New(sha256.New, []byte(secretStr))
			mac.Write(payloadBytes)
			signature := hex.EncodeToString(mac.Sum(nil))
			req.Header.Set("X-Signature", signature)
		}
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	return fmt.Sprintf("status: %d", resp.StatusCode), nil
}

func (s *deliveryService) markFailed(ctx context.Context, log domain.LandingLeadDeliveryLog, err error) {
	errMsg := err.Error()
	var nextRetry *time.Time

	// Retry logic (max 3 attempts)
	if log.Attempts < 2 {
		retryTime := time.Now().Add(time.Minute * 5)
		nextRetry = &retryTime
	}

	tc, _ := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     log.OrganizationID,
		OrganizationSlug:   "worker-process",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceWorker,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	scope, _ := coretenant.NewScope(tc)

	s.integrationRepo.UpdateDeliveryLogStatus(ctx, scope, log.ID, repository.UpdateDeliveryLogParams{
		Status:       domain.DeliveryStatusFailed,
		ErrorMessage: &errMsg,
		NextRetryAt:  nextRetry,
	})
}
