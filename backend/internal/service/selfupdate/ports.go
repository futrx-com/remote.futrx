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
