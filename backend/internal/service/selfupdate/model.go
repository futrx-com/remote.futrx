package selfupdate

// UpdateKind selects the smallest safe deployment path for a release.
// Application updates stay within the installed major/minor release line;
// crossing either boundary requires full infrastructure convergence.
type UpdateKind string

const (
	UpdateKindApplication    UpdateKind = "application"
	UpdateKindInfrastructure UpdateKind = "infrastructure"
)

// CheckResult is the outcome of one tag lookup against origin.
type CheckResult struct {
	CheckedAt       int64      `json:"checkedAt"`
	LatestTag       string     `json:"latestTag,omitempty"`
	UpdateAvailable bool       `json:"updateAvailable"`
	UpdateKind      UpdateKind `json:"updateKind,omitempty"`
	Error           string     `json:"error,omitempty"`
}

// RunStatus describes the most recent apply run, reconstructed from disk so
// it stays accurate across the restart the update itself triggers.
type RunStatus struct {
	State        string     `json:"state"` // running | succeeded | failed
	Target       string     `json:"target"`
	UpdateKind   UpdateKind `json:"updateKind,omitempty"`
	StartedAt    int64      `json:"startedAt"`
	StartedBy    string     `json:"startedBy,omitempty"`
	FinishedAt   int64      `json:"finishedAt,omitempty"`
	ExitCode     *int       `json:"exitCode,omitempty"`
	Log          string     `json:"log,omitempty"`
	LogUpdatedAt int64      `json:"logUpdatedAt,omitempty"`
	Progress     *Progress  `json:"progress,omitempty"`
}

// Progress is the durable, structured view of the updater's current phase.
// The raw log remains available for diagnostics, while these fields let the
// UI explain long-running work without guessing from terminal output.
type Progress struct {
	Phase       string `json:"phase"`
	Message     string `json:"message"`
	Completed   int    `json:"completed,omitempty"`
	Total       int    `json:"total,omitempty"`
	CurrentItem string `json:"currentItem,omitempty"`
	UpdatedAt   int64  `json:"updatedAt"`
}

type Status struct {
	CurrentVersion string       `json:"currentVersion"`
	LastCheck      *CheckResult `json:"lastCheck,omitempty"`
	Run            *RunStatus   `json:"run,omitempty"`
}

type runRecord struct {
	Target     string     `json:"target"`
	UpdateKind UpdateKind `json:"updateKind,omitempty"`
	StartedAt  int64      `json:"startedAt"`
	StartedBy  string     `json:"startedBy"`
	PID        int        `json:"pid"`
}

type doneRecord struct {
	ExitCode   int   `json:"exitCode"`
	FinishedAt int64 `json:"finishedAt"`
}
