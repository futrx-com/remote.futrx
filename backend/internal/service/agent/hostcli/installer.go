// Package hostcli converges the host-side agent CLIs declared by the module
// catalog. Provider profiles own package names, scripts, binaries, and pins;
// this package owns only the common installation policy.
package hostcli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/shared/output"
)

var semanticVersionPattern = regexp.MustCompile(`(?:^|[^0-9A-Za-z])v?([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)(?:$|[^0-9A-Za-z.-])`)

const defaultInstallTimeout = 10 * time.Minute

// Runtime is the host command surface needed by Installer. Keeping command
// execution behind this port makes convergence deterministic in tests.
type Runtime interface {
	CommandExists(binary string) bool
	ExecutablePath(binary string) string
	Version(context.Context, string, ...string) (string, error)
	InstallNPM(context.Context, string, string, string) (string, error)
	InstallScript(context.Context, string) (string, error)
}

// Result describes one converged CLI without exposing provider-specific
// installation details to the infrastructure entry point.
type Result struct {
	Provider        string
	Name            string
	Version         string
	DetectedVersion string
	VersionChecked  bool
	Changed         bool
}

type Installer struct {
	runtime        Runtime
	versionTimeout time.Duration
}

// New creates the host CLI convergence workflow with the application-wide
// deadline used for each version probe.
func New(runtime Runtime, versionTimeout time.Duration) *Installer {
	return &Installer{runtime: runtime, versionTimeout: versionTimeout}
}

// EnsureAll converges every host profile in catalog order. Installations run
// sequentially because npm's global prefix is shared process-wide.
func (i *Installer) EnsureAll(ctx context.Context, profiles []provisioning.Profile) ([]Result, error) {
	if i == nil || i.runtime == nil {
		return nil, errors.New("host agent CLI runtime is unavailable")
	}
	if i.versionTimeout <= 0 {
		return nil, errors.New("host agent CLI version timeout must be positive")
	}
	results := make([]Result, 0, len(profiles))
	for _, profile := range profiles {
		result, err := i.ensure(ctx, profile)
		if err != nil {
			return nil, fmt.Errorf("converge host agent %q: %w", profile.ID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (i *Installer) ensure(ctx context.Context, profile provisioning.Profile) (Result, error) {
	spec := profile.CLI
	result := Result{
		Provider:       profile.ID,
		Name:           spec.Name,
		Version:        spec.Version,
		VersionChecked: spec.CheckVersion,
	}
	ready, detected := i.ready(ctx, spec)
	result.DetectedVersion = detected
	if ready {
		return result, nil
	}

	installCtx, cancel := installContext(ctx, spec.InstallTimeout)
	defer cancel()

	var installOutput string
	var err error
	switch spec.InstallMode {
	case provisioning.InstallWithNPM, provisioning.InstallWithImageRepair:
		// Image repair is a container fallback for old images without npm. The
		// host step runs after Node convergence, so both npm-backed modes use
		// the provider's canonical package directly here.
		if strings.TrimSpace(spec.PackageName) == "" {
			return Result{}, errors.New("npm install policy has no package")
		}
		installOutput, err = i.runtime.InstallNPM(installCtx, spec.PackageName, spec.NPMPackage(), spec.Binary)
	case provisioning.InstallWithScript:
		if strings.TrimSpace(spec.InstallScript) == "" {
			return Result{}, errors.New("script install policy has no script")
		}
		installOutput, err = i.runtime.InstallScript(installCtx, spec.InstallScript)
	default:
		return Result{}, fmt.Errorf("unsupported install mode %q", spec.InstallMode)
	}
	if err != nil {
		return Result{}, fmt.Errorf("install %s: %w; output: %s", installLabel(spec), err, output.TruncateTail(installOutput, 1000))
	}

	result.Changed = true
	// Installation and verification have independent budgets. A successful
	// install that used most of its deadline still receives a complete bounded
	// version probe instead of inheriting an almost-expired install context.
	ready, result.DetectedVersion = i.ready(ctx, spec)
	if spec.VerifyAfterInstall && !ready {
		errorMessage := fmt.Sprintf(
			"install %s completed but the required binary/version is unavailable (detected %q, want %q)",
			installLabel(spec),
			result.DetectedVersion,
			spec.Version,
		)
		if executablePath := i.runtime.ExecutablePath(spec.Binary); executablePath != "" {
			errorMessage += fmt.Sprintf("; PATH resolves %q to %q", spec.Binary, executablePath)
		}
		return Result{}, errors.New(errorMessage)
	}
	return result, nil
}

func (i *Installer) ready(ctx context.Context, spec provisioning.CLISpec) (bool, string) {
	if !spec.CheckVersion {
		return i.runtime.CommandExists(spec.Binary), ""
	}
	versionCtx, cancel := context.WithTimeout(ctx, i.versionTimeout)
	defer cancel()
	versionOutput, err := i.runtime.Version(versionCtx, spec.Binary, spec.VersionArgs...)
	if err != nil {
		return false, ""
	}
	return matchSemanticVersion(versionOutput, spec.Version)
}

func installContext(parent context.Context, timeoutDuration time.Duration) (context.Context, context.CancelFunc) {
	if timeoutDuration <= 0 {
		timeoutDuration = defaultInstallTimeout
	}
	return context.WithTimeout(parent, timeoutDuration)
}

func matchSemanticVersion(versionOutput, want string) (bool, string) {
	matches := semanticVersionPattern.FindAllStringSubmatch(versionOutput, -1)
	detected := ""
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		detected = match[1]
		if detected == want {
			return true, detected
		}
	}
	return false, detected
}

func installLabel(spec provisioning.CLISpec) string {
	if spec.ReportVersion && spec.Version != "" {
		return spec.Name + " " + spec.Version
	}
	return spec.Name
}
