package template

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrMalformedVariable = errors.New("malformed template variable")
	ErrUnknownVariable   = errors.New("unknown template variable")
)

var variableKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type Parser struct {
	registry *VariableRegistry
}

func NewParser(registry *VariableRegistry) *Parser {
	if registry == nil {
		registry = NewVariableRegistry()
	}
	return &Parser{registry: registry}
}

func (p *Parser) Parse(text string) ([]string, error) {
	return parseVariables(text)
}

func (p *Parser) ParseForTemplate(templateCode string, text string) ([]string, error) {
	variables, err := p.Parse(text)
	if err != nil {
		return nil, err
	}

	for _, variable := range variables {
		if !p.registry.Has(templateCode, variable) {
			return nil, fmt.Errorf("%w: %s", ErrUnknownVariable, variable)
		}
	}

	return variables, nil
}

func parseVariables(text string) ([]string, error) {
	variables := make([]string, 0)
	seen := map[string]bool{}

	for offset := 0; offset < len(text); {
		nextOpen := strings.Index(text[offset:], "{{")
		nextClose := strings.Index(text[offset:], "}}")

		if nextClose >= 0 && (nextOpen < 0 || nextClose < nextOpen) {
			return nil, fmt.Errorf("%w: unmatched closing braces", ErrMalformedVariable)
		}
		if nextOpen < 0 {
			break
		}

		start := offset + nextOpen
		endOffset := strings.Index(text[start+2:], "}}")
		if endOffset < 0 {
			return nil, fmt.Errorf("%w: unclosed opening braces", ErrMalformedVariable)
		}

		end := start + 2 + endOffset
		rawKey := text[start+2 : end]
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return nil, fmt.Errorf("%w: empty variable", ErrMalformedVariable)
		}
		if strings.Contains(key, "{{") || strings.Contains(key, "}}") {
			return nil, fmt.Errorf("%w: nested variable %q", ErrMalformedVariable, key)
		}
		if !variableKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("%w: invalid variable %q", ErrMalformedVariable, key)
		}

		if !seen[key] {
			variables = append(variables, key)
			seen[key] = true
		}

		offset = end + 2
	}

	return variables, nil
}
