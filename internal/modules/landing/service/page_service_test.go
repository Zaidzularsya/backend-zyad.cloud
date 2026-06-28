package service

import (
	"errors"
	"net/http"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapPagePersistenceErrorDuplicateSlug(t *testing.T) {
	err := mapPagePersistenceError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "idx_landing_pages_organization_slug_active_unique",
	})

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != "PAGE_SLUG_ALREADY_EXISTS" {
		t.Fatalf("expected PAGE_SLUG_ALREADY_EXISTS, got %s", appErr.Code)
	}
	if appErr.Status != http.StatusConflict {
		t.Fatalf("expected HTTP 409, got %d", appErr.Status)
	}
}

func TestMapPagePersistenceErrorNoRows(t *testing.T) {
	err := mapPagePersistenceError(pgx.ErrNoRows)

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != "PAGE_NOT_FOUND" {
		t.Fatalf("expected PAGE_NOT_FOUND, got %s", appErr.Code)
	}
	if appErr.Status != http.StatusNotFound {
		t.Fatalf("expected HTTP 404, got %d", appErr.Status)
	}
}
