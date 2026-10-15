package service

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
)

// Composer HTML (TipTap) is sanitized before it is stored or sent: the
// stored copy is rendered again in the app, and recipients should not get
// scripts or tracking from a compromised browser either.
var bodyPolicy = func() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()
	policy.RequireNoFollowOnLinks(false)
	policy.AddTargetBlankToFullyQualifiedLinks(true)
	return policy
}()

var (
	stripPolicy    = bluemonday.StrictPolicy()
	blockBreaks    = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|li|h[1-6]|blockquote|tr)>`)
	extraBlankLine = regexp.MustCompile(`\n{3,}`)
	whitespaceRun  = regexp.MustCompile(`\s+`)
)

func sanitizeHTML(body string) string {
	return strings.TrimSpace(bodyPolicy.Sanitize(body))
}

// htmlToText renders the plain-text alternative part.
func htmlToText(body string) string {
	withBreaks := blockBreaks.ReplaceAllString(body, "$0\n")
	text := html.UnescapeString(stripPolicy.Sanitize(withBreaks))
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(extraBlankLine.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

func snippet(text string, max int) string {
	collapsed := strings.TrimSpace(whitespaceRun.ReplaceAllString(text, " "))
	if utf8.RuneCountInString(collapsed) <= max {
		return collapsed
	}
	return string([]rune(collapsed)[:max-1]) + "…"
}
