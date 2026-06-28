package validation

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func TestRegisterGinValidators(t *testing.T) {
	RegisterGinValidators()

	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		t.Fatal("expected gin validator engine")
	}

	type request struct {
		Slug        string `binding:"alphanumhyphen"`
		TrackingKey string `binding:"alphanumunderscore"`
	}

	if err := engine.Struct(request{Slug: "company-profile", TrackingKey: "hero_cta"}); err != nil {
		t.Fatalf("expected valid custom validator values: %v", err)
	}
	if err := engine.Struct(request{Slug: "-bad", TrackingKey: "bad-key"}); err == nil {
		t.Fatal("expected invalid custom validator values")
	}
}
