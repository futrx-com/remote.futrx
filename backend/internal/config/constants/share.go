package constants

import "time"

const (
	// ProjectShareDefaultTTL applies when a caller omits the lifetime.
	ProjectShareDefaultTTL = 24 * time.Hour
	// ProjectShareMinTTL prevents creation of an already-useless grant.
	ProjectShareMinTTL = time.Hour
	// ProjectShareMaxTTL bounds how long an unauthenticated grant may survive.
	ProjectShareMaxTTL = 30 * 24 * time.Hour

	// ProjectShareTokenBytes is the token entropy before base64 encoding.
	ProjectShareTokenBytes = 32
	// ProjectShareMaxLabelLength bounds the operator's reminder text.
	ProjectShareMaxLabelLength = 80
	// ProjectShareMaxPerProject bounds the live grants owned by one project.
	ProjectShareMaxPerProject = 50
)
