package selfupdate

import "context"

// UpdaterLaunch names the complete boundary between update policy and the
// host process that performs the selected release deployment.
type UpdaterLaunch struct {
	InstallDir   string
	Target       string
	Kind         UpdateKind
	LogPath      string
	DonePath     string
	ProgressPath string
}

// HostClient is implemented by integration/updatecli.
type HostClient interface {
	ListRemoteTags(ctx context.Context, installDir string) ([]string, error)
	StartUpdater(launch UpdaterLaunch) (int, error)
	ProcessAlive(pid int) bool
}

// UpdateLifecyclePublisher is the lifecycle notification capability used by
// the self-update workflow. The concrete publisher is supplied at composition.
type UpdateLifecyclePublisher interface {
	PublishUpdateStarted(ctx context.Context, target, kind, startedBy string)
	PublishUpdateSucceeded(ctx context.Context, target, kind, startedBy string)
	PublishUpdateFailed(ctx context.Context, target, kind, startedBy string)
}
