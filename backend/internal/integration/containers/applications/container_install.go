package applications

import (
	"fmt"
	"strings"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// containerBuildScript returns the provisioning program for an application's
// backend/container/ source: fetch a Go toolchain for the container's own
// architecture, build the application's commands, install them, and record the
// build so that doing it again is free.
//
// This is generated rather than shipped because it was identical in every
// application that had one, down to a hand-copied Go version that had already
// drifted between them. An application supplies Go source; the shell is the
// server's to write.
func containerBuildScript(applicationID, buildVersion string, commands []string) []byte {
	marker := fmt.Sprintf("%s/%s.build", configconstants.ApplicationContainerBuildMarkerDir, applicationID)

	var out strings.Builder
	out.WriteString("set -euo pipefail\n")
	fmt.Fprintf(&out, "APP_BUILD_VERSION=%s\n", shellQuote(buildVersion))
	fmt.Fprintf(&out, "APP_BUILD_MARKER=%s\n", shellQuote(marker))
	out.WriteString("export APP_BUILD_VERSION\n")
	out.WriteString(`APP_CONTAINER_SOURCE="${APP_PACKAGE_DIR:?container source was not staged}/` + infraPayloadRoot + `"
export APP_CONTAINER_SOURCE

if [ "$(cat "$APP_BUILD_MARKER" 2>/dev/null || true)" != "$APP_BUILD_VERSION" ]; then
  GOROOT_DIR=/usr/local/go
  if ! "$GOROOT_DIR/bin/go" version 2>/dev/null | grep -q 'go` + configconstants.ApplicationContainerGoVersion + ` '; then
    ARCH="$(dpkg --print-architecture)"
    case "$ARCH" in
      amd64 | arm64) ;;
      *)
        echo "install: unsupported container architecture: $ARCH" >&2
        exit 1
        ;;
    esac
    if ! command -v curl >/dev/null 2>&1; then
      apt-get -o DPkg::Lock::Timeout=300 update -qq
      apt-get -o DPkg::Lock::Timeout=300 install -y -qq --no-install-recommends curl ca-certificates
    fi
    curl -fsSL -o /tmp/remote-go.tgz "https://go.dev/dl/go` + configconstants.ApplicationContainerGoVersion + `.linux-${ARCH}.tar.gz"
    rm -rf "$GOROOT_DIR"
    tar -C /usr/local -xzf /tmp/remote-go.tgz
    rm -f /tmp/remote-go.tgz
  fi

  APP_BUILD_DIR="$(mktemp -d)"
  trap 'rm -rf -- "$APP_BUILD_DIR"' EXIT
`)
	for _, build := range containerBuildTargets(applicationID, commands) {
		output := `"$APP_BUILD_DIR"/` + shellQuote(build.command)
		fmt.Fprintf(&out,
			"  (cd \"$APP_CONTAINER_SOURCE\" && CGO_ENABLED=0 \"$GOROOT_DIR/bin/go\" build"+
				" -mod=mod -buildvcs=false -trimpath"+
				" -ldflags \"-s -w -X main.version=$APP_BUILD_VERSION\""+
				" -o %s %s)\n",
			output, shellQuote(build.target))
		fmt.Fprintf(&out, "  install -m 0755 %s %s\n",
			output, shellQuote("/usr/local/bin/"+build.command))
	}
	fmt.Fprintf(&out, `
  mkdir -p %s
  printf '%%s\n' "$APP_BUILD_VERSION" >"$APP_BUILD_MARKER"
fi
`, shellQuote(configconstants.ApplicationContainerBuildMarkerDir))
	return []byte(out.String())
}

type containerBuildTarget struct {
	command string
	target  string
}

// containerBuildTargets keeps root programs distinct from programs discovered
// below cmd/. A command is allowed to have the same name as its application;
// its source location, not its name, decides which Go package is built.
func containerBuildTargets(applicationID string, commands []string) []containerBuildTarget {
	if len(commands) == 0 {
		return []containerBuildTarget{{command: applicationID, target: "."}}
	}
	targets := make([]containerBuildTarget, 0, len(commands))
	for _, command := range commands {
		targets = append(targets, containerBuildTarget{
			command: command,
			target:  "./cmd/" + command,
		})
	}
	return targets
}

// shellQuote renders a value as a single-quoted shell word. Everything it is
// given here is server-derived, but the binaries and versions it interpolates
// end up in a root shell inside a container, so nothing reaches one unquoted.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
