package project

import "errors"

var (
	ErrReservedNameSeparator = errors.New("project name must not contain consecutive hyphens (--)")
	ErrNameRequired          = errors.New("name is required")
	ErrNameAlreadyExists     = errors.New("project name already exists")
	ErrInvalidID             = errors.New("invalid project id")
	ErrNotFound              = errors.New("project not found")
	ErrInvalidSecretKey      = errors.New("invalid secret key (must match [A-Za-z_][A-Za-z0-9_]*)")
	ErrInvalidLimits         = errors.New("invalid container resource limits")
	ErrSecretsUnavailable    = errors.New("secrets store is not configured")
	// ErrSetupFailed identifies a project script failure after container preparation.
	// Runtime, persistence, and environment-loading failures must not use it.
	ErrSetupFailed = errors.New("project setup failed")
	// ErrInsufficientStorage means the host cannot hold another project
	// container. Callers wrap it with the numbers that explain why.
	ErrInsufficientStorage = errors.New("not enough disk space")
)
