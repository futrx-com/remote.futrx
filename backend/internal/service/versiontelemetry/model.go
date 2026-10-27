package versiontelemetry

import "time"

// Attempt records the last version-report request, whether or not the remote
// collector accepted it.
type Attempt struct {
	Version string
	At      time.Time
}

// Report is the minimal payload sent to the version collector.
type Report struct {
	InstallationID string
	Version        string
}
