package templates

import "context"

// Runtime is the template workflow's container port. Command arguments and
// transport details stay behind it, so the provisioning policy above is
// testable without LXD.
type Runtime interface {
	Available() bool
	// FileExists reports whether an absolute path exists in the container.
	FileExists(ctx context.Context, containerName, path string) (bool, error)
	// EnsureDirectory creates a directory (and parents) in the container.
	EnsureDirectory(ctx context.Context, containerName, path string) error
	// PushFile writes content to an absolute container path with fileMode.
	PushFile(ctx context.Context, containerName string, content []byte, target, fileMode string) error
	// RunScript executes a bash program inside the container and returns its
	// combined output.
	RunScript(ctx context.Context, containerName, script string) (string, error)
	// ImageExists reports whether an image alias is published on this host.
	ImageExists(ctx context.Context, alias string) (bool, error)
}
