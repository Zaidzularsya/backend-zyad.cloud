package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/modules/landing/service"
)

func TestFailMenuItemStatusMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"invalid internal page", service.ErrInvalidInternalPageDestination, http.StatusUnprocessableEntity},
		{"wrapped invalid internal page", fmt.Errorf("wrap: %w", service.ErrInvalidInternalPageDestination), http.StatusUnprocessableEntity},
		{"invalid link type", service.ErrInvalidLinkType, http.StatusUnprocessableEntity},
		{"max depth", service.ErrMaxMenuDepth, http.StatusBadRequest},
		{"unknown", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			failMenuItem(c, tt.err)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}
