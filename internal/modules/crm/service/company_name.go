package service

import (
	"strings"
	"unicode"
)

var companyLegalTokens = map[string]bool{"pt": true, "cv": true, "tbk": true, "ud": true}

// NormalizeCompanyName: huruf kecil, tanda baca → spasi, buang bentuk
// badan usaha (PT, CV, Tbk, UD), spasi dirapikan.
func NormalizeCompanyName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, name)
	words := make([]string, 0, 4)
	for _, w := range strings.Fields(cleaned) {
		if !companyLegalTokens[w] {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

func isSimilarCompanyName(normalizedNeedle, candidate string) bool {
	c := NormalizeCompanyName(candidate)
	return c != "" && (c == normalizedNeedle || strings.Contains(c, normalizedNeedle) || strings.Contains(normalizedNeedle, c))
}
