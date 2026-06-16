package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

const maxImpersonationDuration = 2 * time.Hour

type ImpersonationStore interface {
	Start(
		context.Context,
		repository.StartImpersonationParams,
	) (model.ImpersonationSession, error)
	Stop(
		context.Context,
		repository.StopImpersonationParams,
	) (model.ImpersonationSession, error)
}

type ImpersonationMetadata struct {
	RequestID string
	IPAddress string
	UserAgent string
}

type ImpersonationService struct {
	store ImpersonationStore
	now   func() time.Time
}

func NewImpersonationService(store ImpersonationStore) *ImpersonationService {
	return &ImpersonationService{store: store, now: time.Now}
}

func (s *ImpersonationService) Start(
	ctx context.Context,
	operatorUserID string,
	operatorSessionID string,
	targetOrganizationID string,
	request dto.StartImpersonationRequest,
	metadata ImpersonationMetadata,
) (dto.ImpersonationSessionResponse, error) {
	if !validUUID(operatorUserID) ||
		!validUUID(operatorSessionID) ||
		!validUUID(targetOrganizationID) {
		return dto.ImpersonationSessionResponse{},
			validationError("operator_user_id, operator_session_id, and organization_id must be valid UUIDs")
	}
	targetUserID := strings.TrimSpace(request.TargetUserID)
	if targetUserID != "" && !validUUID(targetUserID) {
		return dto.ImpersonationSessionResponse{},
			validationError("target_user_id must be a valid UUID")
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		return dto.ImpersonationSessionResponse{}, validationError("reason is required")
	}
	expiresAt, err := parseEntitlementTime(request.ExpiresAt, "expires_at")
	if err != nil {
		return dto.ImpersonationSessionResponse{}, err
	}
	now := s.now().UTC()
	if !expiresAt.After(now) {
		return dto.ImpersonationSessionResponse{},
			validationError("expires_at must be in the future")
	}
	if expiresAt.After(now.Add(maxImpersonationDuration)) {
		return dto.ImpersonationSessionResponse{},
			validationError("expires_at must be within two hours")
	}
	if s.store == nil {
		return dto.ImpersonationSessionResponse{}, impersonationStoreRequiredError()
	}
	session, err := s.store.Start(ctx, repository.StartImpersonationParams{
		OperatorSessionID:    strings.TrimSpace(operatorSessionID),
		OperatorUserID:       strings.TrimSpace(operatorUserID),
		TargetOrganizationID: strings.TrimSpace(targetOrganizationID),
		TargetUserID:         targetUserID,
		Reason:               reason,
		TicketReference:      request.TicketReference,
		ExpiresAt:            expiresAt,
		Metadata:             request.Metadata,
		RequestID:            metadata.RequestID,
		IPAddress:            metadata.IPAddress,
		UserAgent:            metadata.UserAgent,
		StartedAt:            now,
	})
	if err != nil {
		return dto.ImpersonationSessionResponse{}, mapImpersonationError(
			"IMPERSONATION_START_FAILED",
			"failed to start organization impersonation",
			err,
		)
	}
	return impersonationResponse(session, s.now().UTC()), nil
}

func (s *ImpersonationService) Stop(
	ctx context.Context,
	operatorUserID string,
	operatorSessionID string,
	request dto.StopImpersonationRequest,
	metadata ImpersonationMetadata,
) (dto.ImpersonationSessionResponse, error) {
	if !validUUID(operatorUserID) || !validUUID(operatorSessionID) {
		return dto.ImpersonationSessionResponse{},
			validationError("operator_user_id and operator_session_id must be valid UUIDs")
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		return dto.ImpersonationSessionResponse{}, validationError("reason is required")
	}
	if s.store == nil {
		return dto.ImpersonationSessionResponse{}, impersonationStoreRequiredError()
	}
	stoppedAt := s.now().UTC()
	session, err := s.store.Stop(ctx, repository.StopImpersonationParams{
		OperatorSessionID: strings.TrimSpace(operatorSessionID),
		OperatorUserID:    strings.TrimSpace(operatorUserID),
		Reason:            reason,
		RequestID:         metadata.RequestID,
		IPAddress:         metadata.IPAddress,
		UserAgent:         metadata.UserAgent,
		StoppedAt:         stoppedAt,
	})
	if err != nil {
		return dto.ImpersonationSessionResponse{}, mapImpersonationError(
			"IMPERSONATION_STOP_FAILED",
			"failed to stop organization impersonation",
			err,
		)
	}
	return impersonationResponse(session, stoppedAt), nil
}

func impersonationResponse(
	session model.ImpersonationSession,
	now time.Time,
) dto.ImpersonationSessionResponse {
	return dto.ImpersonationSessionResponse{
		ID:                   session.ID,
		OperatorSessionID:    session.OperatorSessionID,
		OperatorUserID:       session.OperatorUserID,
		TargetOrganizationID: session.TargetOrganizationID,
		TargetUserID:         session.TargetUserID,
		Reason:               session.Reason,
		TicketReference:      session.TicketReference,
		StartedAt:            session.StartedAt.UTC().Format(time.RFC3339),
		ExpiresAt:            session.ExpiresAt.UTC().Format(time.RFC3339),
		StoppedAt:            formatOrganizationTime(session.StoppedAt),
		StoppedByUserID:      session.StoppedByUserID,
		StopReason:           session.StopReason,
		Metadata:             nonNilLimits(session.Metadata),
		IsActive:             session.IsActive(now),
	}
}

func impersonationStoreRequiredError() error {
	return coreerrors.New(
		"IMPERSONATION_STORE_REQUIRED",
		"impersonation store is required",
		http.StatusInternalServerError,
	)
}

func mapImpersonationError(code, message string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New(
			"IMPERSONATION_NOT_FOUND",
			"active impersonation or target organization was not found",
			http.StatusNotFound,
		)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return coreerrors.New(
				"IMPERSONATION_ALREADY_ACTIVE",
				"operator session already has an active impersonation",
				http.StatusConflict,
			)
		case "23503", "P0001":
			return coreerrors.New(
				"IMPERSONATION_TARGET_INVALID",
				"impersonation target is invalid",
				http.StatusConflict,
			)
		}
	}
	return coreerrors.Wrap(code, message, http.StatusInternalServerError, err)
}
