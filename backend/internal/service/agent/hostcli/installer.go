// Package hostcli converges the host-side agent CLIs declared by the module
// catalog. Provider profiles own package names, scripts, binaries, and pins;
// this package owns only the common installation policy.
package hostcli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
	CommandExists(executablePath string) bool
	ExecutablePath(binary string) string
	Version(context.Context, string, ...string) (string, error)
	InstallNPM(context.Context, string, string) (string, error)
	InstallScript(context.Context, string, string) (string, error)
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
	managedPrefix  string
}

// New creates the host CLI convergence workflow with the application-wide
// deadline used for each version probe and the application-owned prefix where
// every host CLI must be installed. Keeping all installer modes behind one
// prefix makes installation, verification, and runtime PATH resolution agree.
func New(runtime Runtime, versionTimeout time.Duration, managedPrefix string) *Installer {
	return &Installer{
		runtime:        runtime,
		versionTimeout: versionTimeout,
		managedPrefix:  filepath.Clean(managedPrefix),
	}
}

// EnsureAll converges every host profile in catalog order. Installations run
// sequentially because every installer mode shares one managed prefix.
func (i *Installer) EnsureAll(ctx context.Context, profiles []provisioning.Profile) ([]Result, error) {
	if i == nil || i.runtime == nil {
		return nil, errors.New("host agent CLI runtime is unavailable")
	}
	if i.versionTimeout <= 0 {
		return nil, errors.New("host agent CLI version timeout must be positive")
	}
	if !filepath.IsAbs(i.managedPrefix) || i.managedPrefix == string(filepath.Separator) {
		return nil, errors.New("host agent CLI managed prefix must be an absolute path below root")
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
	if spec.Binary == "" || filepath.Base(spec.Binary) != spec.Binary || spec.Binary == "." || spec.Binary == ".." {
		return Result{}, errors.New("host agent CLI binary must be a plain file name")
	}
	managedExecutable := filepath.Join(i.managedPrefix, "bin", spec.Binary)
	result := Result{
		Provider:       profile.ID,
		Name:           spec.Name,
		Version:        spec.Version,
		VersionChecked: spec.CheckVersion,
	}
	ready, detected := i.ready(ctx, spec, managedExecutable)
	result.DetectedVersion = detected
	if ready {
		if err := i.requireManagedResolution(spec.Binary, managedExecutable); err != nil {
			return Result{}, err
		}
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
		installOutput, err = i.runtime.InstallNPM(installCtx, i.managedPrefix, spec.NPMPackage())
	case provisioning.InstallWithScript:
		if strings.TrimSpace(spec.InstallScript) == "" {
			return Result{}, errors.New("script install policy has no script")
		}
		installOutput, err = i.runtime.InstallScript(installCtx, spec.InstallScript, managedExecutable)
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
	ready, result.DetectedVersion = i.ready(ctx, spec, managedExecutable)
	if spec.VerifyAfterInstall && !ready {
		return Result{}, fmt.Errorf(
			"install %s completed but the managed executable %q has the wrong or unavailable version (detected %q, want %q)",
			installLabel(spec),
			managedExecutable,
			result.DetectedVersion,
			spec.Version,
		)
	}
	if err := i.requireManagedResolution(spec.Binary, managedExecutable); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (i *Installer) ready(ctx context.Context, spec provisioning.CLISpec, managedExecutable string) (bool, string) {
	if !spec.CheckVersion {
		return i.runtime.CommandExists(managedExecutable), ""
	}
	versionCtx, cancel := context.WithTimeout(ctx, i.versionTimeout)
	defer cancel()
	versionOutput, err := i.runtime.Version(versionCtx, managedExecutable, spec.VersionArgs...)
	if err != nil {
		return false, ""
	}
	return matchSemanticVersion(versionOutput, spec.Version)
}

func (i *Installer) requireManagedResolution(binary, managedExecutable string) error {
	resolvedExecutable := i.runtime.ExecutablePath(binary)
	if filepath.Clean(resolvedExecutable) == filepath.Clean(managedExecutable) {
		return nil
	}
	if resolvedExecutable == "" {
		resolvedExecutable = "not found"
	}
	return fmt.Errorf(
		"managed executable %q is installed but PATH resolves %q to %q",
		managedExecutable,
		binary,
		resolvedExecutable,
	)
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
