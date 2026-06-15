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
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
	SortOrder   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
