package applications

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

const defaultInstallScriptPath = "infra/install.sh"

// applicationInfrastructure is the complete container-side capability derived
// from one application package. Keeping the correlated values together avoids
// positional return values drifting as capability discovery evolves.
type applicationInfrastructure struct {
	installPath string
	script      []byte
	container   *svc.ApplicationContainer
}

// validateInstallScriptPath rejects manifest overrides outside the capability's
// directory. It runs before other capability discovery so load errors retain
// their established order.
func validateInstallScriptPath(scriptPath string) error {
	if scriptPath != "" && (!fs.ValidPath(scriptPath) || !strings.HasPrefix(scriptPath, "infra/")) {
		return fmt.Errorf("install script must be inside infra/")
	}
	return nil
}

// loadApplicationInfrastructure discovers and prepares an application's
// container-side capability. An explicit manifest path is required to exist.
// backend/container/ generates a build script and may stand alone or run before
// a custom infra script.
func loadApplicationInfrastructure(catalog fs.FS, root, applicationID, applicationVersion, configuredPath string) (applicationInfrastructure, error) {
	installPath := configuredPath
	if installPath == "" {
		installPath = defaultInstallScriptPath
	}

	script, err := fs.ReadFile(catalog, path.Join(root, installPath))
	if errors.Is(err, fs.ErrNotExist) && configuredPath == "" {
		script = nil
		installPath = ""
		err = nil
	}
	if err != nil {
		return applicationInfrastructure{}, fmt.Errorf("read install script %q: %w", installPath, err)
	}

	container, err := packContainerSource(catalog, root, applicationID)
	if err != nil {
		return applicationInfrastructure{}, fmt.Errorf("container source: %w", err)
	}
	if container == nil {
		script, err = withInfraPayload(catalog, root, script)
		if err != nil {
			return applicationInfrastructure{}, fmt.Errorf("infra payload: %w", err)
		}
		return applicationInfrastructure{
			installPath: installPath,
			script:      script,
		}, nil
	}
	if _, err := fs.Stat(catalog, path.Join(root, "infra", "payload.tar.gz")); err == nil {
		return applicationInfrastructure{}, fmt.Errorf("backend/container and infra/payload.tar.gz cannot both be present")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return applicationInfrastructure{}, fmt.Errorf("inspect infra payload: %w", err)
	}

	buildVersion := deriveContainerBuildVersion(applicationVersion, container.digest)
	prologue := containerBuildScript(applicationID, buildVersion, container.commands)
	combined := append(prologue, script...)
	return applicationInfrastructure{
		installPath: installPath,
		script:      stageInfraPayload(container.payload, combined),
		container: &svc.ApplicationContainer{
			Commands:     append([]string(nil), container.commands...),
			SourceDigest: container.digest,
			BuildVersion: buildVersion,
		},
	}, nil
}

func deriveContainerBuildVersion(applicationVersion, digest string) string {
	if applicationVersion == "" {
		return digest[:16]
	}
	return applicationVersion + "+" + digest[:16]
}
