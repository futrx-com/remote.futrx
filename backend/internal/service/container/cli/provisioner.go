// Package cli coordinates agent CLI readiness, installation, and repair
// without depending on LXD command details.
package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	"github.com/futrx-com/remote.futrx.com/internal/shared/output"
)

const failedInstallWaitTimeout = 90 * time.Second

// Runtime translates CLI queries and mutations into container operations.
type Runtime interface {
	Available() bool
	Version(ctx context.Context, containerName, binary string, arguments ...string) (string, error)
	CommandExists(ctx context.Context, containerName, command string) bool
	InstallRunning(ctx context.Context, containerName, packageName string) bool
	InstallNPM(ctx context.Context, containerName, npmPackage string) (string, error)
	Repair(ctx context.Context, containerName, script string) (string, error)
}

// Provisioner owns agent CLI readiness policy, installation sequencing, and
// coalescing around installs already running inside a container.
type Provisioner struct {
	runtime      Runtime
	profiles     serviceprofiles.Source
	repairRecipe func([]provisioning.Profile) (string, error)
}

func NewProvisioner(
	runtime Runtime,
	profileSource serviceprofiles.Source,
	repairRecipe func([]provisioning.Profile) (string, error),
) *Provisioner {
	return &Provisioner{
		runtime:      runtime,
		profiles:     profileSource,
		repairRecipe: repairRecipe,
	}
}

// Ensure is cheap on the normal path. Missing or stale CLIs are upgraded to
// the repository pin, while concurrent prompt starts wait for an install that
// is already running before starting another one.
func (p *Provisioner) Ensure(ctx context.Context, containerName string, spec provisioning.CLISpec) error {
	if !p.runtime.Available() {
		return errors.New("lxc not available")
	}
	if p.ready(ctx, containerName, spec) {
		return nil
	}
	if spec.PackageName != "" && p.runtime.InstallRunning(ctx, containerName, spec.PackageName) {
		waitCtx, cancel := context.WithTimeout(ctx, spec.WaitTimeout)
		defer cancel()
		if err := p.waitUntilReady(waitCtx, containerName, spec); err == nil {
			return nil
		}
	}

	installCtx, cancel := context.WithTimeout(ctx, spec.InstallTimeout)
	defer cancel()

	var out string
	var err error
	if spec.InstallMode == provisioning.InstallWithScript {
		if spec.InstallScript == "" {
			return fmt.Errorf("agent CLI %s uses script install but has no install script", spec.Name)
		}
		out, err = p.runtime.Repair(installCtx, containerName, spec.InstallScript)
	} else if spec.InstallMode == provisioning.InstallWithImageRepair {
		installScript, scriptErr := p.repairRecipe(p.profiles.Snapshot())
		if scriptErr != nil {
			return fmt.Errorf("prepare agent CLI repair: %w", scriptErr)
		}
		out, err = p.runtime.Repair(installCtx, containerName, installScript)
	} else if p.runtime.CommandExists(installCtx, containerName, "npm") {
		out, err = p.runtime.InstallNPM(installCtx, containerName, spec.NPMPackage())
	} else {
		// Very old containers may pre-date Node/npm. Reuse the full image recipe
		// in that case so the runtime still self-heals from a bare rootfs.
		installScript, scriptErr := p.repairRecipe(p.profiles.Snapshot())
		if scriptErr != nil {
			return fmt.Errorf("prepare agent CLI repair: %w", scriptErr)
		}
		out, err = p.runtime.Repair(installCtx, containerName, installScript)
	}
	if err != nil {
		waitCtx, cancelWait := context.WithTimeout(ctx, failedInstallWaitTimeout)
		defer cancelWait()
		if waitErr := p.waitUntilReady(waitCtx, containerName, spec); waitErr == nil {
			return nil
		}
		return fmt.Errorf("install %s in %s: %w; output: %s",
			cliInstallLabel(spec), containerName, err, output.TruncateTail(out, 1000))
	}
	if spec.VerifyAfterInstall && !p.ready(ctx, containerName, spec) {
		return fmt.Errorf("install %s in %s completed but the required version is unavailable",
			cliInstallLabel(spec), containerName)
	}
	return nil
}

func cliInstallLabel(spec provisioning.CLISpec) string {
	if spec.ReportVersion && spec.Version != "" {
		return spec.Name + " " + spec.Version
	}
	return spec.Name
}

func (p *Provisioner) ready(ctx context.Context, containerName string, spec provisioning.CLISpec) bool {
	if !spec.CheckVersion {
		return p.runtime.CommandExists(ctx, containerName, spec.Binary)
	}
	out, err := p.runtime.Version(ctx, containerName, spec.Binary, spec.VersionArgs...)
	return err == nil && semanticVersionAtLeast(out, spec.Version)
}

func (p *Provisioner) waitUntilReady(ctx context.Context, containerName string, spec provisioning.CLISpec) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if p.ready(ctx, containerName, spec) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
