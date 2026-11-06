// Package config owns the container terminal's defaults and resource limits.
package config

import "time"

const (
	DefaultSocket      = "/run/remote-terminal/terminal.sock"
	WorkspaceDirectory = "/workspace"

	DetachedTTL  = 10 * time.Minute
	MaxSessions  = 64
	ReplayBytes  = 256 << 10
	PollWait     = 20 * time.Second
	ReapInterval = 30 * time.Second

	ReplayLineSearchBytes = 4 << 10
	HangupGrace           = 2 * time.Second
	MaxWindowCells        = 1000
	PTYReadBufferBytes    = 8192

	// Remote forwards at most 1 MiB per backend call.
	MaxBodyBytes             = 1 << 20
	MaxWorkingDirectoryBytes = 4096
	ReadHeaderTimeout        = 5 * time.Second

	// A 1 MiB body base64-encodes to under 1.4 MiB in a bridge line.
	MaxBridgeFrameBytes    = 4 << 20
	MaxBridgeResponseBytes = 2 << 20
	BridgeIdleConnections  = 16
)
