package validation

import (
	"regexp"
	"sync"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var (
	registerGinValidatorsOnce sync.Once
	alphanumHyphenPattern     = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	alphanumUnderscorePattern = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*$`)
)

func RegisterGinValidators() {
	registerGinValidatorsOnce.Do(func() {
		engine, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}

		_ = engine.RegisterValidation("alphanumhyphen", validateAlphanumHyphen)
		_ = engine.RegisterValidation("alphanumunderscore", validateAlphanumUnderscore)
	})
}

func validateAlphanumHyphen(field validator.FieldLevel) bool {
	return alphanumHyphenPattern.MatchString(field.Field().String())
}

func validateAlphanumUnderscore(field validator.FieldLevel) bool {
	return alphanumUnderscorePattern.MatchString(field.Field().String())
}
