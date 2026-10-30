package service

import (
	"errors"
	"strings"
	"unicode/utf8"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/repository"
)

// Aturan tulis form & field (spec Form Konsultasi Fleksibel §4.2). Hanya berlaku saat data
// ditulis; form yang sudah ada tidak dimigrasi.

var (
	ErrCoreFieldsRequired    = errors.New("LANDING_FORM_CORE_FIELDS")
	ErrInvalidDefaultValue   = errors.New("invalid field default value")
	ErrInvalidWhatsAppNumber = errors.New("invalid whatsapp number")
	ErrInvalidAfterSubmit    = errors.New("invalid after_submit")
)

// CoreFieldKeys wajib ada, tampil, dan wajib diisi pada form yang meneruskan ke CRM.
var CoreFieldKeys = []string{"name", "email", "phone"}

const (
	maxFieldDefaultValueRunes = 500
	minWhatsAppDigits         = 8
	maxWhatsAppDigits         = 15
)

// NormalizeWhatsAppNumber membuang spasi, '-', '(', ')', '+' lalu mengganti awalan 0 menjadi 62.
// "" berarti nomor dikosongkan. Hasil harus 8-15 digit.
func NormalizeWhatsAppNumber(raw string) (string, error) {
	cleaned := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", "+", "").Replace(strings.TrimSpace(raw))
	if cleaned == "" {
		return "", nil
	}
	for _, r := range cleaned {
		if r < '0' || r > '9' {
			return "", ErrInvalidWhatsAppNumber
		}
	}
	// Panjang minimum dicek pada digit yang diketik penulis ("0812345" = 7 digit ditolak, spec §9);
	// panjang maksimum pada hasil akhir, karena awalan 0 -> 62 menambah satu digit.
	if len(cleaned) < minWhatsAppDigits {
		return "", ErrInvalidWhatsAppNumber
	}
	if strings.HasPrefix(cleaned, "0") {
		cleaned = "62" + cleaned[1:]
	}
	if len(cleaned) > maxWhatsAppDigits {
		return "", ErrInvalidWhatsAppNumber
	}
	return cleaned, nil
}

// normalizeFieldParams menormalisasi dan memvalidasi daftar field sebelum disimpan.
func normalizeFieldParams(in []repository.CreateFormFieldParams) ([]repository.CreateFormFieldParams, error) {
	out := make([]repository.CreateFormFieldParams, len(in))
	for i, p := range in {
		if p.Type == domain.FormFieldTypeHidden {
			p.IsRequired = false
		}
		p.DefaultValue = strings.TrimSpace(p.DefaultValue)
		if utf8.RuneCountInString(p.DefaultValue) > maxFieldDefaultValueRunes {
			return nil, ErrInvalidDefaultValue
		}
		if p.DefaultValue != "" && isSelectLike(p) && !containsString(p.Options, p.DefaultValue) {
			return nil, ErrInvalidDefaultValue
		}
		out[i] = p
	}
	return out, nil
}

// isSelectLike: select, atau field yang disembunyikan dari select (tipe asli di validation.original_type).
func isSelectLike(p repository.CreateFormFieldParams) bool {
	if p.Type == domain.FormFieldTypeSelect {
		return true
	}
	orig, _ := p.Validation["original_type"].(string)
	return p.Type == domain.FormFieldTypeHidden && orig == string(domain.FormFieldTypeSelect)
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

type fieldShape struct {
	Key        string
	Type       domain.FormFieldType
	IsRequired bool
}

// checkCoreFields memastikan name/email/phone ada, tidak hidden, dan wajib bila form meneruskan ke CRM.
// Form tanpa field sama sekali dianggap sedang disusun.
func checkCoreFields(createCRMLead bool, fields []fieldShape) error {
	if !createCRMLead || len(fields) == 0 {
		return nil
	}
	byKey := make(map[string]fieldShape, len(fields))
	for _, f := range fields {
		byKey[f.Key] = f
	}
	for _, key := range CoreFieldKeys {
		f, ok := byKey[key]
		if !ok || f.Type == domain.FormFieldTypeHidden || !f.IsRequired {
			return ErrCoreFieldsRequired
		}
	}
	return nil
}

// checkAfterSubmit memastikan aksi setelah kirim lengkap: redirect butuh URL valid, whatsapp butuh nomor.
func checkAfterSubmit(action domain.AfterSubmitAction, redirectURL, whatsappNumber string) error {
	if !action.IsValid() {
		return ErrInvalidAfterSubmit
	}
	switch action {
	case domain.AfterSubmitRedirect:
		if redirectURL == "" || dto.ValidateRedirectURL(redirectURL) != nil {
			return ErrInvalidAfterSubmit
		}
	case domain.AfterSubmitWhatsApp:
		if whatsappNumber == "" {
			return ErrInvalidAfterSubmit
		}
	}
	return nil
}

func shapesFromParams(in []repository.CreateFormFieldParams) []fieldShape {
	out := make([]fieldShape, len(in))
	for i, p := range in {
		out[i] = fieldShape{Key: p.Key, Type: p.Type, IsRequired: p.IsRequired}
	}
	return out
}

func shapesFromFields(in []domain.LandingFormField) []fieldShape {
	out := make([]fieldShape, len(in))
	for i, f := range in {
		out[i] = fieldShape{Key: f.Key, Type: f.Type, IsRequired: f.IsRequired}
	}
	return out
}
