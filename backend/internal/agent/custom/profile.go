package custom

// Package custom adapts an admin-supplied AI provider (API key + base URL)
// as a headless agent provider. Unlike the CLI-backed providers there is no
// host CLI, no OAuth handshake, and no container credential sync: the admin
// enters the configuration once and it is persisted to disk. Chat execution
// wiring is out of scope for this change; the auth gate is the load-bearing
// part. The profile is intentionally minimal — no CLI, no credentials —
// mirroring the lightweight antigravity case.

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

var customProfile = provisioning.Profile{
	ID:          string(agent.ProviderCustom),
	Credentials: provisioning.CredentialSpec{Name: "custom"},
}

// Profile returns the custom provider's container-facing policy as a
// defensive copy. There is no CLI to install and no credential directory to
// sync; the admin-supplied API key lives on the host only.
func Profile() provisioning.Profile {
	return customProfile.Clone()
}
