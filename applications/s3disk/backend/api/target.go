package api

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// containerName is the shape Remote gives a project container, and the shape
// lxc accepts. Nothing in a request reaches it — this guards against the
// backend being initialized somewhere it cannot work.
var containerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

// backendTarget is the validated, immutable container-side configuration for
// one backend process. Constructing it once keeps handlers from independently
// interpreting instance environment values.
type backendTarget struct {
	instance   applications.Instance
	mountpoint string
	uploadsDir string
	binary     string
	service    string
	// asyncWriteback mirrors the mount flag of the same name. With it set, a
	// closed file is not yet on S3, which decides whether push may delete the
	// copy Remote left behind.
	asyncWriteback bool
}

func newBackendTarget(instance applications.Instance) (backendTarget, error) {
	if instance.Scope != "project" || !containerName.MatchString(instance.ContainerName) {
		return backendTarget{}, fmt.Errorf("s3disk requires a project container")
	}
	if !containerName.MatchString(instance.ApplicationID) {
		return backendTarget{}, fmt.Errorf("s3disk requires Remote application metadata")
	}
	if instance.Service == "" {
		return backendTarget{}, fmt.Errorf("s3disk requires application.json to declare a service")
	}
	mountpoint := instance.Env["S3DISK_MOUNTPOINT"]
	if mountpoint == "" {
		return backendTarget{}, fmt.Errorf("S3DISK_MOUNTPOINT was not resolved from application.json")
	}
	if !filepath.IsAbs(mountpoint) {
		return backendTarget{}, fmt.Errorf("s3disk mountpoint must be absolute")
	}
	uploadsDir, err := uploadsDirOf(instance.Env["S3DISK_UPLOADS_DIR"])
	if err != nil {
		return backendTarget{}, err
	}
	return backendTarget{
		instance:       instance,
		mountpoint:     filepath.Clean(mountpoint),
		uploadsDir:     uploadsDir,
		binary:         path.Join("/usr/local/bin", instance.ApplicationID),
		service:        instance.Service + ".service",
		asyncWriteback: hasMountFlag(instance.Env["S3DISK_MOUNT_ARGS"], "--async-writeback"),
	}, nil
}

// hasMountFlag reports whether the operator passed a flag through
// S3DISK_MOUNT_ARGS. It matches the flag as a whole word so "--async-writeback"
// is not found inside "--no-async-writeback".
func hasMountFlag(args, flag string) bool {
	for _, field := range strings.Fields(args) {
		if field == flag || strings.HasPrefix(field, flag+"=") {
			return true
		}
	}
	return false
}

// uploadsDirOf resolves where inside the bucket pushed attachments land. It is
// an operator setting, not a caller's, but it still ends up in a command line:
// "." and "/" mean the mount root, and anything else has to be a plain
// relative path so it cannot climb out of the mount or look like a flag.
func uploadsDirOf(configured string) (string, error) {
	configured = strings.Trim(strings.TrimSpace(configured), "/")
	switch configured {
	case "":
		return "", fmt.Errorf("S3DISK_UPLOADS_DIR was not resolved from application.json")
	case ".":
		return "", nil
	}
	for _, segment := range strings.Split(configured, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, "-") {
			return "", fmt.Errorf("S3DISK_UPLOADS_DIR must be a plain relative path")
		}
	}
	return configured, nil
}
