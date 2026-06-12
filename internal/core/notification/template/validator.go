package template

import (
	"fmt"
	"regexp"
	"strings"

	"zyad.cloud/internal/core/notification/domain"
)

var (
	templateCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
	localePattern       = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)
)

type FieldError struct {
	Field   string
	Message string
}

type ValidationError struct {
	Fields []FieldError
}

func (e ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "template validation failed"
	}
	return fmt.Sprintf("template validation failed: %s", e.Fields[0].Message)
}

type Validator struct {
	registry *VariableRegistry
	parser   *Parser
}

func NewValidator(registry *VariableRegistry) *Validator {
	if registry == nil {
		registry = NewVariableRegistry()
	}
	return &Validator{
		registry: registry,
		parser:   NewParser(registry),
	}
}

func (v *Validator) Validate(template domain.NotificationTemplate) error {
	var fieldErrors []FieldError
	addError := func(field string, message string) {
		fieldErrors = append(fieldErrors, FieldError{Field: field, Message: message})
	}

	code := strings.TrimSpace(template.Code)
	if !templateCodePattern.MatchString(code) {
		addError("code", "code must use domain.action format")
	}

	if !template.Channel.IsValid() {
		addError("channel", "channel is not supported")
	}

	locale := strings.TrimSpace(template.Locale)
	if locale != "" && !localePattern.MatchString(locale) {
		addError("locale", "locale must use xx or xx-XX format")
	}

	subject := strings.TrimSpace(template.SubjectTemplate)
	if template.Channel == domain.ChannelEmail && subject == "" {
		addError("subject_template", "email template requires subject_template")
	}

	body := strings.TrimSpace(template.BodyTemplate)
	if body == "" {
		addError("body_template", "body_template cannot be empty")
	}

	registryVariables := v.registry.Get(code)
	if len(registryVariables) == 0 {
		addError("code", "template code is not registered")
	} else {
		fieldErrors = append(fieldErrors, v.validateAvailableVariables(registryVariables, template.AvailableVariables)...)
	}

	if subject != "" {
		if _, err := v.parser.ParseForTemplate(code, subject); err != nil {
			addError("subject_template", err.Error())
		}
	}
	if body != "" {
		if _, err := v.parser.ParseForTemplate(code, body); err != nil {
			addError("body_template", err.Error())
		}
	}

	if len(fieldErrors) > 0 {
		return ValidationError{Fields: fieldErrors}
	}
	return nil
}

func (v *Validator) validateAvailableVariables(registryVariables []domain.NotificationVariable, availableVariables []domain.NotificationVariable) []FieldError {
	var fieldErrors []FieldError

	registeredByKey := make(map[string]domain.NotificationVariable, len(registryVariables))
	for _, variable := range registryVariables {
		registeredByKey[variable.Key] = variable
	}

	availableByKey := make(map[string]domain.NotificationVariable, len(availableVariables))
	for _, variable := range availableVariables {
		key := strings.TrimSpace(variable.Key)
		if key == "" {
			fieldErrors = append(fieldErrors, FieldError{Field: "available_variables", Message: "available variable key cannot be empty"})
			continue
		}
		if !variableKeyPattern.MatchString(key) {
			fieldErrors = append(fieldErrors, FieldError{Field: "available_variables", Message: fmt.Sprintf("available variable %q has invalid key format", key)})
			continue
		}
		if _, exists := registeredByKey[key]; !exists {
			fieldErrors = append(fieldErrors, FieldError{Field: "available_variables", Message: fmt.Sprintf("available variable %q is not registered", key)})
			continue
		}
		availableByKey[key] = variable
	}

	for _, variable := range registryVariables {
		if !variable.Required {
			continue
		}
		if _, exists := availableByKey[variable.Key]; !exists {
			fieldErrors = append(fieldErrors, FieldError{Field: "available_variables", Message: fmt.Sprintf("required variable %q is missing", variable.Key)})
		}
	}

	return fieldErrors
}
