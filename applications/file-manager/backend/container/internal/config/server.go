package config

import "time"

const (
	WorkspaceDirectory = "/workspace"
	DefaultPort        = 8844
	ReadHeaderTimeout  = 10 * time.Second
	IdleTimeout        = 60 * time.Second
)
