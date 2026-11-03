package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func recoveryEngine(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery(nil))
	r.GET("/", handler)
	return r
}

func TestRecovery_PanicAnswersInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	recoveryEngine(func(*gin.Context) { panic("boom") }).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("status %d body %s", w.Code, w.Body)
	}
}

// http.ErrAbortHandler is net/http's "abort this response" sentinel: Recovery
// re-panics it without writing a body or logging a stack trace.
func TestRecovery_ErrAbortHandlerPassesThroughQuietly(t *testing.T) {
	var logged bytes.Buffer
	prev := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logged
	defer func() { gin.DefaultErrorWriter = prev }()

	w := httptest.NewRecorder()
	func() {
		defer func() {
			if rec := recover(); rec != http.ErrAbortHandler {
				t.Fatalf("recovered %v", rec)
			}
		}()
		recoveryEngine(func(c *gin.Context) {
			c.String(http.StatusOK, "partial")
			panic(http.ErrAbortHandler)
		}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	}()
	if w.Body.String() != "partial" || logged.Len() != 0 {
		t.Fatalf("body %q logged %q", w.Body, logged.String())
	}
}
