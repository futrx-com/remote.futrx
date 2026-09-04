package project

import "errors"

var (
	ErrNameRequired       = errors.New("name is required")
	ErrNameAlreadyExists  = errors.New("project name already exists")
	ErrInvalidID          = errors.New("invalid project id")
	ErrNotFound           = errors.New("project not found")
	ErrInvalidSecretKey   = errors.New("invalid secret key (must match [A-Za-z_][A-Za-z0-9_]*)")
	ErrInvalidLimits      = errors.New("invalid container resource limits")
	ErrSecretsUnavailable = errors.New("secrets store is not configured")
)
