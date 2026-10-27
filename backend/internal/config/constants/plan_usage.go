package constants

import "time"

const (
	// PlanUsageRefreshInterval is how long plan limits read from the providers
	// stay current. Usage requests within it share the last answer instead of
	// asking every provider account again.
	PlanUsageRefreshInterval = time.Minute
	// PlanUsageReadTimeout bounds one read of every provider account.
	PlanUsageReadTimeout = 30 * time.Second
	// PlanUsageAccountReadTimeout bounds one provider account's network and CLI
	// exchange within the aggregate read.
	PlanUsageAccountReadTimeout = 15 * time.Second
	// ClaudePlanUsageExitGrace and CodexPlanUsageExitGrace let each CLI exit
	// after answering before its process group is stopped.
	ClaudePlanUsageExitGrace = 2 * time.Second
	CodexPlanUsageExitGrace  = time.Second
)
