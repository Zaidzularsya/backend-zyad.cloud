package template

import (
	"errors"
	"testing"

	"zyad.cloud/internal/core/notification/domain"
)

func TestValidatorAcceptsValidEmailTemplate(t *testing.T) {
	validator := NewValidator(nil)

	err := validator.Validate(validPasswordResetTemplate())
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidatorRejectsEmailWithoutSubject(t *testing.T) {
	validator := NewValidator(nil)
	template := validPasswordResetTemplate()
	template.SubjectTemplate = ""

	err := validator.Validate(template)
	assertValidationField(t, err, "subject_template")
}

func TestValidatorRejectsEmptyBody(t *testing.T) {
	validator := NewValidator(nil)
	template := validPasswordResetTemplate()
	template.BodyTemplate = "  "

	err := validator.Validate(template)
	assertValidationField(t, err, "body_template")
}

func TestValidatorRejectsInvalidCodeFormat(t *testing.T) {
	validator := NewValidator(nil)
	template := validPasswordResetTemplate()
	template.Code = "password-reset"

	err := validator.Validate(template)
	assertValidationField(t, err, "code")
}

func TestValidatorRejectsInvalidChannel(t *testing.T) {
	validator := NewValidator(nil)
	template := validPasswordResetTemplate()
	template.Channel = domain.Channel("sms")

	err := validator.Validate(template)
	assertValidationField(t, err, "channel")
}

func TestValidatorRejectsInvalidLocale(t *testing.T) {
	validator := NewValidator(nil)
	template := validPasswordResetTemplate()
	template.Locale = "id_id"

	err := validator.Validate(template)
	assertValidationField(t, err, "locale")
}

func TestValidatorRejectsUnknownUsedVariable(t *testing.T) {
	validator := NewValidator(nil)
	template := validPasswordResetTemplate()
	template.BodyTemplate = "Hello {{unknown_variable}}"

	err := validator.Validate(template)
	assertValidationField(t, err, "body_template")
}

func TestValidatorRejectsMissingRequiredAvailableVariable(t *testing.T) {
	validator := NewValidator(nil)
	template := validPasswordResetTemplate()
	template.AvailableVariables = template.AvailableVariables[:3]

	err := validator.Validate(template)
	assertValidationField(t, err, "available_variables")
}

func TestValidatorRejectsUnregisteredAvailableVariable(t *testing.T) {
	validator := NewValidator(nil)
	template := validPasswordResetTemplate()
	template.AvailableVariables = append(template.AvailableVariables, domain.NotificationVariable{
		Key:      "custom_variable",
		Required: false,
	})

	err := validator.Validate(template)
	assertValidationField(t, err, "available_variables")
}

func validPasswordResetTemplate() domain.NotificationTemplate {
	return domain.NotificationTemplate{
		Code:            "auth.password_reset",
		Name:            "Password Reset",
		Channel:         domain.ChannelEmail,
		Locale:          "id-ID",
		SubjectTemplate: "Reset password for {{app_name}}",
		BodyTemplate:    "Hello {{user_name}}, open {{reset_url}} before {{expired_at}}.",
		AvailableVariables: []domain.NotificationVariable{
			{Key: "app_name", Description: "Application name", Required: true},
			{Key: "user_name", Description: "Recipient user name", Required: true},
			{Key: "reset_url", Description: "Password reset URL", Required: true},
			{Key: "expired_at", Description: "Reset link expiration time", Required: true},
		},
	}
}

func assertValidationField(t *testing.T, err error, field string) {
	t.Helper()

	var validationError ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("Validate() error = %v, want ValidationError", err)
	}
	for _, fieldError := range validationError.Fields {
		if fieldError.Field == field {
			return
		}
	}
	t.Fatalf("Validate() fields = %#v, want field %q", validationError.Fields, field)
}
