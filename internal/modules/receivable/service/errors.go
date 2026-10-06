package service

import "fmt"

// validationError membawa pesan Indonesia yang aman ditampilkan ke pengguna dan
// cocok dengan sentinel-nya lewat errors.Is (ErrInvalidInvoice, ErrInvalidAccount, ErrInvalidSettings).
type validationError struct {
	kind   error
	detail string
}

func (e *validationError) Error() string        { return e.detail }
func (e *validationError) Is(target error) bool { return target == e.kind }

func invalidInvoice(format string, args ...any) error {
	return &validationError{kind: ErrInvalidInvoice, detail: fmt.Sprintf(format, args...)}
}

func invalidAccount(format string, args ...any) error {
	return &validationError{kind: ErrInvalidAccount, detail: fmt.Sprintf(format, args...)}
}

func invalidSettings(format string, args ...any) error {
	return &validationError{kind: ErrInvalidSettings, detail: fmt.Sprintf(format, args...)}
}
