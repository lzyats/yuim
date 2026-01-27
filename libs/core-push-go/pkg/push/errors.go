package push

import "errors"

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotConfigured   = errors.New("not configured")
)
