package template

import (
	"errors"
	"testing"
)

func TestParserParseExtractsUniqueVariables(t *testing.T) {
	parser := NewParser(nil)

	variables, err := parser.Parse("Hello {{user_name}}, reset at {{ reset_url }}. Again {{user_name}}.")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := []string{"user_name", "reset_url"}
	if len(variables) != len(want) {
		t.Fatalf("Parse() length = %d, want %d", len(variables), len(want))
	}
	for i := range want {
		if variables[i] != want[i] {
			t.Fatalf("Parse()[%d] = %q, want %q", i, variables[i], want[i])
		}
	}
}

func TestParserParseIgnoresNormalText(t *testing.T) {
	parser := NewParser(nil)

	variables, err := parser.Parse("Plain notification without variables.")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(variables) != 0 {
		t.Fatalf("Parse() length = %d, want 0", len(variables))
	}
}

func TestParserParseRejectsMalformedVariables(t *testing.T) {
	parser := NewParser(nil)

	tests := []string{
		"Hello {{user_name",
		"Hello user_name}}",
		"Hello {{}}",
		"Hello {{user-name}}",
		"Hello {{UserName}}",
		"Hello {{user {{name}} }}",
	}

	for _, tt := range tests {
		if _, err := parser.Parse(tt); !errors.Is(err, ErrMalformedVariable) {
			t.Fatalf("Parse(%q) error = %v, want ErrMalformedVariable", tt, err)
		}
	}
}

func TestParserParseForTemplateRejectsUnknownVariable(t *testing.T) {
	parser := NewParser(nil)

	_, err := parser.ParseForTemplate("auth.password_reset", "Hello {{unknown_variable}}")
	if !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("ParseForTemplate() error = %v, want ErrUnknownVariable", err)
	}
}

func TestParserParseForTemplateAllowsKnownVariables(t *testing.T) {
	parser := NewParser(nil)

	variables, err := parser.ParseForTemplate("auth.password_reset", "Hello {{user_name}}, open {{reset_url}}")
	if err != nil {
		t.Fatalf("ParseForTemplate() error = %v", err)
	}
	if len(variables) != 2 {
		t.Fatalf("ParseForTemplate() length = %d, want 2", len(variables))
	}
}
