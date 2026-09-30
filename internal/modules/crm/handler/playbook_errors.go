package handler

import (
	"errors"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/crm/playbook"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
)

// mapPlaybookError turns playbook/lead-lifecycle sentinel errors into API
// errors; other errors pass through unchanged.
func mapPlaybookError(err error) error {
	switch {
	case errors.Is(err, playbook.ErrOutcomeRequired):
		return coreerrors.New("OUTCOME_REQUIRED", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, playbook.ErrOutcomeInvalid):
		return coreerrors.New("OUTCOME_INVALID", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, playbook.ErrOutcomeInputRequired):
		return coreerrors.New("OUTCOME_INPUT_REQUIRED", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, service.ErrPlaybookStepCancelNotAllowed):
		return coreerrors.New("PLAYBOOK_STEP_CANCEL_NOT_ALLOWED", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, service.ErrDueAtRequired):
		return coreerrors.New("DUE_AT_REQUIRED", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, service.ErrInvalidActivityStatus):
		return coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, repository.ErrPlaybookDisabled):
		return coreerrors.New("PLAYBOOK_DISABLED", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, repository.ErrPlaybookAlreadyActive):
		return coreerrors.New("PLAYBOOK_ALREADY_ACTIVE", err.Error(), http.StatusConflict)
	case errors.Is(err, repository.ErrPlaybookNotApplicable):
		return coreerrors.New("PLAYBOOK_NOT_APPLICABLE", err.Error(), http.StatusConflict)
	case errors.Is(err, repository.ErrPlaybookRunNotActive):
		return coreerrors.New("ACTIVITY_NOT_PENDING", err.Error(), http.StatusNotFound)
	}
	return err
}
