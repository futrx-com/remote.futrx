//go:build linux

package config

import "time"

const (
	// ContainerCommand is the program backend/container/ builds into the
	// project container. Its service owns every shell; its bridge relays this
	// backend's requests to that service.
	ContainerCommand = "/usr/local/bin/remote-terminal"

	// CallTimeout bounds one relayed request. Output reads wait up to 20
	// seconds inside the container, and the manifest allows 30 per call.
	CallTimeout = 25 * time.Second
)
