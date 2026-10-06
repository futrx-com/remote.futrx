package models

import "errors"

var (
	ErrInvalidScope      = errors.New("invalid permission scope")
	ErrInvalidDefinition = errors.New("invalid permission definition")
	ErrInvalidState      = errors.New("invalid permission state")
)
