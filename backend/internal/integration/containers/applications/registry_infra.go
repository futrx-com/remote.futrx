package applications

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

const defaultInstallScriptPath = "infra/install.sh"

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
// container-side capability. An explicit manifest path is required to exist;
// an absent conventional path means the application has no infrastructure.
func loadApplicationInfrastructure(catalog fs.FS, root, configuredPath string) (string, []byte, error) {
	installPath := configuredPath
	if installPath == "" {
		installPath = defaultInstallScriptPath
	}

	script, err := fs.ReadFile(catalog, path.Join(root, installPath))
	if errors.Is(err, fs.ErrNotExist) && configuredPath == "" {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("read install script %q: %w", installPath, err)
	}
	script, err = withInfraPayload(catalog, root, script)
	if err != nil {
		return "", nil, fmt.Errorf("infra payload: %w", err)
	}
	return installPath, script, nil
}
