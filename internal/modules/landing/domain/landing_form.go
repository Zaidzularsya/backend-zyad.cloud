package domain

import "time"

type FormFieldType string

const (
	FormFieldTypeText     FormFieldType = "text"
	FormFieldTypeTextarea FormFieldType = "textarea"
	FormFieldTypeEmail    FormFieldType = "email"
	FormFieldTypePhone    FormFieldType = "phone"
	FormFieldTypeNumber   FormFieldType = "number"
	FormFieldTypeSelect   FormFieldType = "select"
	FormFieldTypeRadio    FormFieldType = "radio"
	FormFieldTypeCheckbox FormFieldType = "checkbox"
	FormFieldTypeFile     FormFieldType = "file"
	FormFieldTypeDate     FormFieldType = "date"
	FormFieldTypeHidden   FormFieldType = "hidden"
)

func (t FormFieldType) IsValid() bool {
	switch t {
	case FormFieldTypeText,
		FormFieldTypeTextarea,
		FormFieldTypeEmail,
		FormFieldTypePhone,
		FormFieldTypeNumber,
		FormFieldTypeSelect,
		FormFieldTypeRadio,
		FormFieldTypeCheckbox,
		FormFieldTypeFile,
		FormFieldTypeDate,
		FormFieldTypeHidden:
		return true
	default:
		return false
	}
}

// AfterSubmitAction menentukan apa yang terjadi di browser setelah form berhasil dikirim.
type AfterSubmitAction string

const (
	AfterSubmitMessage  AfterSubmitAction = "message"
	AfterSubmitRedirect AfterSubmitAction = "redirect"
	AfterSubmitWhatsApp AfterSubmitAction = "whatsapp"
)

func (a AfterSubmitAction) IsValid() bool {
	switch a {
	case AfterSubmitMessage, AfterSubmitRedirect, AfterSubmitWhatsApp:
		return true
	default:
		return false
	}
}

type LandingForm struct {
	ID             string
	LandingPageID  string
	Name           string
	Key            string
	Description    string
	SubmitLabel    string
	SuccessMessage string
	RedirectURL    string
	IsActive       bool
	// CreateCRMLead: submission form ini boleh dijadikan lead CRM.
	CreateCRMLead bool
	// LeadOwnerUserID: PIC lead; kosong -> pembuat halaman (landing_pages.created_by).
	LeadOwnerUserID string
	// AfterSubmit: pesan sukses, redirect, atau lanjut ke WhatsApp.
	AfterSubmit     AfterSubmitAction
	WhatsAppNumber  string // digit saja, format internasional tanpa '+'
	WhatsAppMessage string // template pesan; kosong = template default di frontend
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Fields          []LandingFormField
}

type LandingFormField struct {
	ID          string
	FormID      string
	Key         string
	Type        FormFieldType
	Label       string
	Placeholder string
	Options     []string
	Validation  map[string]any
	IsRequired  bool
	// DefaultValue dipakai server untuk field hidden bila pengunjung tidak mengirim nilai.
	DefaultValue string
	SortOrder    int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
