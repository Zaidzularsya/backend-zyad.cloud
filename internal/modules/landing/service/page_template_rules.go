package service

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/landing/domain"
)

var pageTemplateSlugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

const catalogPricingSlot = "catalog-pricing"

func templateValidationError(msg string) error {
	return coreerrors.New("VALIDATION_ERROR", msg, http.StatusUnprocessableEntity)
}

// ValidatePageTemplateMeta memvalidasi metadata template (spec §5.1).
func ValidatePageTemplateMeta(name, slug, category, description string, sortOrder int) error {
	if n := utf8.RuneCountInString(slug); n < 1 || n > 80 || !pageTemplateSlugPattern.MatchString(slug) {
		return templateValidationError("slug harus 1–80 karakter: huruf kecil, angka, dan tanda hubung")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(name)); n < 1 || n > 120 {
		return templateValidationError("nama harus 1–120 karakter")
	}
	if utf8.RuneCountInString(description) > 240 {
		return templateValidationError("deskripsi maksimal 240 karakter")
	}
	if !domain.IsValidPageTemplateCategory(category) {
		return templateValidationError("kategori tidak valid")
	}
	if sortOrder < 0 || sortOrder > 9999 {
		return templateValidationError("urutan harus 0–9999")
	}
	return nil
}

// LockPlatformOnly memaksa platform_only bila markup memakai slot katalog (K13).
func LockPlatformOnly(markup string, requested bool) bool {
	if requested {
		return true
	}
	return strings.Contains(markup, `data-zyad-slot="`+catalogPricingSlot+`"`)
}

func isTenantSentinel(n *html.Node) bool {
	for _, a := range n.Attr {
		if a.Key == "data-zyad-slot" && (a.Val == "tenant-nav" || a.Val == "tenant-footer") {
			return true
		}
	}
	return false
}

// IsEmptyTemplateMarkup true bila tidak ada elemen selain sentinel header/footer
// tenant dan teks whitespace.
func IsEmptyTemplateMarkup(markup string) bool {
	nodes, err := html.ParseFragment(strings.NewReader(markup), &html.Node{
		Type: html.ElementNode, Data: "div", DataAtom: atom.Div,
	})
	if err != nil {
		return false
	}
	var hasContent func(n *html.Node) bool
	hasContent = func(n *html.Node) bool {
		switch n.Type {
		case html.TextNode:
			return strings.TrimSpace(n.Data) != ""
		case html.ElementNode:
			if isTenantSentinel(n) {
				return false
			}
			return true
		}
		return false
	}
	for _, n := range nodes {
		if hasContent(n) {
			return false
		}
	}
	return true
}

// NextWorkPageSlug membentuk "template-<slug>", lalu "-2", "-3", … sampai
// taken(slug) false.
func NextWorkPageSlug(base string, taken func(string) bool) string {
	candidate := "template-" + base
	if !taken(candidate) {
		return candidate
	}
	for i := 2; ; i++ {
		c := candidate + "-" + strconv.Itoa(i)
		if !taken(c) {
			return c
		}
	}
}
