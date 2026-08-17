package project

import "errors"

var (
	ErrNameRequired       = errors.New("name is required")
	ErrInvalidID          = errors.New("invalid project id")
	ErrNotFound           = errors.New("project not found")
	ErrInvalidSecretKey   = errors.New("invalid secret key (must match [A-Za-z_][A-Za-z0-9_]*)")
	ErrInvalidLimits      = errors.New("invalid container resource limits")
	ErrSecretsUnavailable = errors.New("secrets store is not configured")
	ErrUnknownTemplate    = errors.New("unknown project template")
)
