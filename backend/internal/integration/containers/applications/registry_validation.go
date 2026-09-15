package applications

import (
	"fmt"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/integration/hosttools"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func validateApplication(application svc.Application) error {
	if len(application.HostTools) > 0 && !application.NeedsContainer() {
		return fmt.Errorf("host tools require a provisioned application")
	}
	for _, tool := range application.HostTools {
		if err := hosttools.Validate(tool); err != nil {
			return fmt.Errorf("host tool %q: %w", tool.Name, err)
		}
	}

	if application.Name == "" {
		return fmt.Errorf("missing name")
	}
	// Version is what an installed instance is compared against to decide
	// whether its install script has to run again, so an application without one
	// could never be upgraded in place. It is free text — "8.0", "16",
	// "1.2.3-rc1" — because the only question ever asked of it is whether it
	// differs from what an instance recorded, never which of two is newer.
	if strings.TrimSpace(application.Version) == "" {
		return fmt.Errorf("missing version")
	}
	if len(application.Scopes) == 0 {
		return fmt.Errorf("missing scopes")
	}
	for _, scope := range application.Scopes {
		if !scope.Valid() {
			return fmt.Errorf("invalid scope %q", scope)
		}
	}
	if !application.NeedsContainer() {
		if application.Port.Internal != 0 || application.Port.DefaultExternal != 0 || application.Healthcheck.Command != "" || application.Service != "" {
			return fmt.Errorf("port, healthcheck, and service require infra/install.sh")
		}
	} else if application.Port.Internal == 0 && (application.Port.DefaultExternal != 0 || application.Healthcheck.Command != "") {
		return fmt.Errorf("port.defaultExternal and healthcheck require port.internal")
	}
	if !application.NeedsContainer() && len(application.Skills) == 0 {
		return fmt.Errorf("application has no infra or skills")
	}
	return nil
}
