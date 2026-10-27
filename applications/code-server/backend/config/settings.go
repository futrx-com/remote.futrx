package config

import "time"

const (
	MaxSettingsBytes = 128 << 10
	ContainerTimeout = 15 * time.Second
	ActiveSettings   = "/root/.local/share/code-server/User/settings.json"
)
