package applications

import (
	"context"
	"fmt"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"strings"
)

// Uninstall removes the proxy device and, for global scope, deletes the
// dedicated container outright. For project scope it stops the service and
// runs optional application-owned cleanup before removing Remote's unit files.
func (in *Installer) Uninstall(ctx context.Context, spec svc.InstallSpec) error {
	inst := spec.Instance
	if inst.Scope == svc.ScopeGlobal {
		// A failed legacy install may have been persisted before container target
		// resolution completed. There is no container footprint to remove in that
		// case, and passing an empty name to LXD turns Retry into a permanent error.
		if inst.ContainerName == "" {
			return nil
		}
		// Deleting the container also drops its proxy device.
		if _, err := command.RunWithTimeout(ctx, in.runner, launchTimeout, "delete", "--force", inst.ContainerName); err != nil {
			if !isMissing(err, "") {
				return fmt.Errorf("delete app container %s: %w", inst.ContainerName, err)
			}
		}
		return nil
	}
	if err := in.removeDevice(ctx, inst.ContainerName, inst.DeviceName); err != nil {
		return err
	}
	if svcName := spec.Application.ServiceName(); svcName != "" {
		_, _ = in.exec(ctx, inst.ContainerName, nil, controlTimeout, "systemctl", "disable", "--now", svcName)
	}
	if err := in.runProjectUninstallScript(ctx, spec); err != nil {
		return err
	}
	if spec.Application.ServiceName() != "" {
		in.removeServiceFiles(ctx, spec)
	}
	in.removeSkills(ctx, spec)
	return nil
}

// runProjectUninstallScript removes application-owned files after its service
// is stopped. A missing container has no files to remove, while a stopped one
// must be started explicitly before cleanup can run inside it.
func (in *Installer) runProjectUninstallScript(ctx context.Context, spec svc.InstallSpec) error {
	script, ok := in.registry.UninstallScript(spec.Application.ID)
	if !ok || spec.Instance.ContainerName == "" {
		return nil
	}
	container := spec.Instance.ContainerName
	state, err := in.containerState(ctx, container)
	if err != nil {
		return err
	}
	switch state {
	case "running":
		out, err := in.execStdin(ctx, container, in.scriptEnv(spec), execTimeout,
			strings.NewReader(string(script)), "bash", "-s")
		if err != nil {
			return fmt.Errorf("uninstall %s: %w; output: %s", spec.Application.ID, err, tail(out))
		}
	case "missing":
		// A replaced project container has no package or settings to remove.
	default:
		return fmt.Errorf("start project container %s before uninstalling %s", container, spec.Application.ID)
	}
	return nil
}
