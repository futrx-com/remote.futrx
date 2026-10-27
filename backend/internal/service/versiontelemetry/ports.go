package versiontelemetry

import "context"

// Repository owns the stable installation identity and report-attempt state.
type Repository interface {
	InstallationID() (string, error)
	LastAttempt() (Attempt, bool, error)
	SaveAttempt(Attempt) error
}

// Reporter is the outbound capability used to publish one pseudonymous
// running-version heartbeat. Implementations must bound their own I/O.
type Reporter interface {
	ReportVersion(ctx context.Context, report Report) error
}
