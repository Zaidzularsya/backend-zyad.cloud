package dispatcher

import "errors"

var (
	ErrInvalidMessage = errors.New("invalid notification message")
	ErrProviderFailed = errors.New("notification provider failed")
)
