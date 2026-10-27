package applications

import (
	"context"
	"io"
)

// InstallProgress describes observed work, rather than an estimated percentage.
// Counts are populated only for an installation across multiple projects.
type InstallProgress struct {
	Phase       string `json:"phase"`
	Message     string `json:"message"`
	CurrentItem string `json:"currentItem,omitempty"`
	Completed   int    `json:"completed,omitempty"`
	Total       int    `json:"total,omitempty"`
}

type installObserverKey struct{}

// ReportInstallProgress lets provisioning adapters report actual phase changes.
// It is a no-op for synchronous callers that did not request observation.
func ReportInstallProgress(ctx context.Context, phase, message string) {
	if observer, ok := ctx.Value(installObserverKey{}).(*observedInstall); ok {
		observer.progress(InstallProgress{Phase: phase, Message: message})
	}
}

// InstallOutput is the redacting, bounded log sink for an observed install.
func InstallOutput(ctx context.Context) io.Writer {
	if observer, ok := ctx.Value(installObserverKey{}).(*observedInstall); ok {
		return observer
	}
	return io.Discard
}

// RedactInstallText protects captured error output before an adapter truncates it.
func RedactInstallText(ctx context.Context, text string) string {
	if observer, ok := ctx.Value(installObserverKey{}).(*observedInstall); ok {
		return observer.redact(text)
	}
	return text
}

func protectInstallInputs(ctx context.Context, application Application, env map[string]string) {
	if observer, ok := ctx.Value(installObserverKey{}).(*observedInstall); ok {
		observer.protect(application, env)
	}
}

func reportInstallProject(ctx context.Context, project string, completed, total int) {
	if observer, ok := ctx.Value(installObserverKey{}).(*observedInstall); ok {
		observer.progress(InstallProgress{
			Phase: "projects", Message: "Installing application in project containers",
			CurrentItem: project, Completed: completed, Total: total,
		})
	}
}
