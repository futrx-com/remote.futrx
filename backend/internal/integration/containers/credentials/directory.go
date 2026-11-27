package credentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

// directorySynchronizer owns dynamic credential directories whose
// provider may add or rotate an unknown set of files.
type directorySynchronizer struct {
	runner command.Runner
	files  *fileSynchronizer
}

func (s *directorySynchronizer) ensure(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error {
	if !s.runner.Available() {
		return command.ErrUnavailable
	}
	directory := spec.Directory
	files, err := regularCredentialFiles(directory.HostPath)
	if err != nil || len(files) == 0 {
		if directory.AllowContainerOnly && s.containerHasFiles(ctx, containerName, directory.ContainerPath) {
			return nil
		}
		if directory.MissingErrorFormat != "" {
			return errors.New(fmt.Sprintf(directory.MissingErrorFormat, containerName))
		}
		return fmt.Errorf("credential directory %s has no files", directory.HostPath)
	}

	pctx, cancel := context.WithTimeout(ctx, authPushTimeout)
	defer cancel()
	for _, path := range directory.ContainerDirs {
		if out, err := s.runner.Run(pctx, "exec", containerName, "--", "install", "-d", "-m", "700", path); err != nil {
			return fmt.Errorf("mkdir %s in container: %w; output: %s", path, err, out)
		}
	}
	for _, name := range files {
		file := provisioning.CredentialFile{
			HostPath:      filepath.Join(directory.HostPath, name),
			ContainerPath: directory.ContainerPath + "/" + name,
			Mode:          "600",
		}
		if err := s.files.pushIfNewer(pctx, file, containerName); err != nil {
			return fmt.Errorf("push %s: %w", file.ContainerPath, err)
		}
	}
	return nil
}

func (s *directorySynchronizer) syncFromContainer(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error {
	directory := spec.Directory
	if !s.runner.Available() {
		if directory.SyncUnavailableIsNoop {
			return nil
		}
		return command.ErrUnavailable
	}
	if directory.SyncOnlyWhenHostHasFiles {
		if files, err := regularCredentialFiles(directory.HostPath); err != nil || len(files) == 0 {
			return nil
		}
	}

	pctx, cancel := context.WithTimeout(ctx, authPushTimeout)
	defer cancel()
	out, err := s.runner.Run(pctx, "exec", containerName, "--",
		"find", directory.ContainerPath, "-maxdepth", "1", "-type", "f", "-printf", "%f\n")
	if err != nil {
		return nil
	}
	for _, name := range strings.Fields(out) {
		containerPath := directory.ContainerPath + "/" + name
		hostPath := filepath.Join(directory.HostPath, name)
		if out, err := s.runner.Run(pctx, "file", "pull", containerName+containerPath, hostPath); err != nil {
			return fmt.Errorf("pull %s: %w; output: %s", containerPath, err, out)
		}
		_ = os.Chmod(hostPath, 0o600)
		now := time.Now()
		_ = os.Chtimes(hostPath, now, now)
	}
	return nil
}

func (s *directorySynchronizer) containerHasFiles(ctx context.Context, containerName, containerPath string) bool {
	out, err := command.RunWithTimeout(ctx, s.runner, queryTimeout, "exec", containerName, "--",
		"sh", "-c", "ls -1 "+containerPath+" 2>/dev/null | head -1")
	return err == nil && strings.TrimSpace(out) != ""
}

func regularCredentialFiles(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}
