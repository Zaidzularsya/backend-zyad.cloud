package template

import (
	"errors"
	"fmt"
	"strings"

	"zyad.cloud/internal/core/notification/domain"
)

var ErrMissingRequiredVariable = errors.New("missing required template variable")

type RenderedTemplate struct {
	Subject string
	Body    string
}

type Renderer struct {
	registry  *VariableRegistry
	parser    *Parser
	validator *Validator
}

func NewRenderer(registry *VariableRegistry) *Renderer {
	if registry == nil {
		registry = NewVariableRegistry()
	}
	return &Renderer{
		registry:  registry,
		parser:    NewParser(registry),
		validator: NewValidator(registry),
	}
}

func (r *Renderer) Render(template domain.NotificationTemplate, payload map[string]any) (RenderedTemplate, error) {
	if err := r.validator.Validate(template); err != nil {
		return RenderedTemplate{}, err
	}
	if payload == nil {
		payload = map[string]any{}
	}

	if err := r.validateRequiredPayload(template.Code, payload); err != nil {
		return RenderedTemplate{}, err
	}

	subject, err := r.renderText(template.Code, template.SubjectTemplate, payload)
	if err != nil {
		return RenderedTemplate{}, err
	}

	body, err := r.renderText(template.Code, template.BodyTemplate, payload)
	if err != nil {
		return RenderedTemplate{}, err
	}

	return RenderedTemplate{
		Subject: subject,
		Body:    body,
	}, nil
}

func (r *Renderer) Preview(template domain.NotificationTemplate, payload map[string]any) (RenderedTemplate, error) {
	if payload == nil {
		payload = template.SamplePayload
	}
	return r.Render(template, payload)
}

func (r *Renderer) validateRequiredPayload(templateCode string, payload map[string]any) error {
	for _, variable := range r.registry.Get(templateCode) {
		if !variable.Required {
			continue
		}
		value, exists := payload[variable.Key]
		if !exists || valueToString(value) == "" {
			return fmt.Errorf("%w: %s", ErrMissingRequiredVariable, variable.Key)
		}
	}
	return nil
}

func (r *Renderer) renderText(templateCode string, text string, payload map[string]any) (string, error) {
	variables, err := r.parser.ParseForTemplate(templateCode, text)
	if err != nil {
		return "", err
	}

	rendered := text
	for _, variable := range variables {
		rendered = replaceVariable(rendered, variable, valueToString(payload[variable]))
	}

	return rendered, nil
}

func replaceVariable(text string, key string, value string) string {
	var builder strings.Builder

	for offset := 0; offset < len(text); {
		startOffset := strings.Index(text[offset:], "{{")
		if startOffset < 0 {
			builder.WriteString(text[offset:])
			break
		}

		start := offset + startOffset
		endOffset := strings.Index(text[start+2:], "}}")
		if endOffset < 0 {
			builder.WriteString(text[offset:])
			break
		}

		end := start + 2 + endOffset
		rawKey := text[start+2 : end]
		if strings.TrimSpace(rawKey) == key {
			builder.WriteString(text[offset:start])
			builder.WriteString(value)
		} else {
			builder.WriteString(text[offset : end+2])
		}
		offset = end + 2
	}

	return builder.String()
}

func valueToString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}
