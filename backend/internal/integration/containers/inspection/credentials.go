package inspection

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// containerCredentialInspector compares each configured credential file's host
// and in-container timestamps.
type containerCredentialInspector struct {
	commands *quickCommandRunner
	profiles serviceprofiles.Source
}

func (i *containerCredentialInspector) inspect(ctx context.Context, containerName string, state serviceproject.ContainerState) []serviceproject.AuthBundleStatus {
	profiles := i.profiles.Snapshot()
	bundles := make([]serviceproject.AuthBundleStatus, 0, len(profiles))
	for _, profile := range profiles {
		credentials := profile.Credentials
		if credentials.Empty() {
			continue
		}
		status := serviceproject.AuthBundleStatus{
			Name:  credentials.Name,
			Files: make([]serviceproject.AuthBundleFileStatus, 0, len(credentials.Files)),
		}
		for _, file := range credentials.Files {
			status.Files = append(status.Files, i.inspectFile(ctx, containerName, state, file.HostPath, file.ContainerPath))
		}
		if credentials.Directory != nil {
			for _, name := range i.credentialDirectoryFiles(ctx, containerName, state, *credentials.Directory) {
				status.Files = append(status.Files, i.inspectFile(
					ctx,
					containerName,
					state,
					filepath.Join(credentials.Directory.HostPath, name),
					path.Join(credentials.Directory.ContainerPath, name),
				))
			}
		}
		bundles = append(bundles, status)
	}
	return bundles
}

func (i *containerCredentialInspector) inspectFile(
	ctx context.Context,
	containerName string,
	state serviceproject.ContainerState,
	hostPath string,
	containerPath string,
) serviceproject.AuthBundleFileStatus {
	status := serviceproject.AuthBundleFileStatus{HostPath: hostPath, ContainerPath: containerPath}
	if info, err := os.Stat(hostPath); err == nil && info.Mode().IsRegular() {
		status.HostExists = true
		status.HostMTime = info.ModTime().Unix()
	}
	if state == serviceproject.ContainerStateRunning {
		if raw, err := i.commands.run(ctx,
			"exec", containerName, "--", "stat", "-c", "%Y", containerPath); err == nil {
			if modifiedAt, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); parseErr == nil {
				status.ContainerExists = true
				status.ContainerMTime = modifiedAt
			}
		}
	}
	switch {
	case status.HostExists && status.ContainerExists:
		status.HostNewer = status.HostMTime > status.ContainerMTime
		status.ContainerNewer = status.ContainerMTime > status.HostMTime
	case status.HostExists && !status.ContainerExists && state == serviceproject.ContainerStateRunning:
		status.HostNewer = true
	case !status.HostExists && status.ContainerExists:
		status.ContainerNewer = true
	}
	return status
}

func (i *containerCredentialInspector) credentialDirectoryFiles(
	ctx context.Context,
	containerName string,
	state serviceproject.ContainerState,
	directory provisioning.CredentialDirectory,
) []string {
	names := make(map[string]bool)
	if entries, err := os.ReadDir(directory.HostPath); err == nil {
		for _, entry := range entries {
			info, infoErr := entry.Info()
			if infoErr == nil && info.Mode().IsRegular() {
				names[entry.Name()] = true
			}
		}
	}
	if state == serviceproject.ContainerStateRunning {
		raw, err := i.commands.run(
			ctx,
			"exec", containerName, "--", "find", directory.ContainerPath,
			"-mindepth", "1", "-maxdepth", "1", "-type", "f", "-printf", "%f\\n",
		)
		if err == nil {
			for _, name := range strings.Split(raw, "\n") {
				name = strings.TrimSpace(name)
				if name != "" && path.Base(name) == name && name != "." && name != ".." {
					names[name] = true
				}
			}
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	return ordered
}
