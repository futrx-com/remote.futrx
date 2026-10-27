package applications

import (
	"fmt"
	"strings"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// Update these with ApplicationContainerGoVersion. Module ZIP hashes were calculated after
// go mod download verified the toolchain modules against sum.golang.org.
var containerGoPins = map[string]string{
	"amd64": "4c2a63c008dbb90112f22ff9e47eb6fb487b17da1c70ecc7b9fad77577b8031e",
	"arm64": "67d836d2f0e17ce538757ba43439fa8f77eb00fe9798e0b0920d94925184e7a6",
}

func containerGoBootstrap(goRoot string, pins map[string]string) string {
	var cases strings.Builder
	for _, arch := range []string{"amd64", "arm64"} {
		fmt.Fprintf(&cases, "      %s) MODULE_SHA=%s ;;\n", arch, shellQuote(pins[arch]))
	}
	return strings.NewReplacer(
		"__GO_ROOT__", shellQuote(goRoot),
		"__GO_VERSION__", shellQuote(configconstants.ApplicationContainerGoVersion),
		"__PINS__", cases.String(),
	).Replace(containerGoBootstrapTemplate)
}

const containerGoBootstrapTemplate = `
  GOROOT_DIR=__GO_ROOT__
  GO_VERSION=__GO_VERSION__
  if ! "$GOROOT_DIR/bin/go" version 2>/dev/null | grep -q "go${GO_VERSION} "; then
    install_application_go() (
      set -euo pipefail
      ARCH="$(dpkg --print-architecture)"
      case "$ARCH" in
__PINS__
        *) echo "install: unsupported container architecture: $ARCH" >&2; exit 1 ;;
      esac
      if ! command -v curl >/dev/null 2>&1 || ! command -v flock >/dev/null 2>&1; then
        apt-get -o DPkg::Lock::Timeout=300 update -qq
        apt-get -o DPkg::Lock::Timeout=300 install -y -qq --no-install-recommends curl ca-certificates util-linux
      fi
      # Different applications share this compiler. Recheck after taking the
      # lock so concurrent installs never move a toolchain another is using.
      exec 9>"$(dirname "$GOROOT_DIR")/.remote-go-install.lock"
      flock 9
      if "$GOROOT_DIR/bin/go" version 2>/dev/null | grep -q "go${GO_VERSION} "; then return 0; fi
      GO_STAGE="$(mktemp -d "$(dirname "$GOROOT_DIR")/.remote-go.XXXXXX")"
      cleanup_go_stage() {
        if [ -e "$GO_STAGE/previous" ] && [ ! -e "$GOROOT_DIR" ]; then
          mv "$GO_STAGE/previous" "$GOROOT_DIR" || return
        fi
        rm -rf -- "$GO_STAGE"
      }
      trap cleanup_go_stage EXIT
      trap 'exit 130' INT
      trap 'exit 143' TERM
      if ! command -v unzip >/dev/null 2>&1; then
        apt-get -o DPkg::Lock::Timeout=300 update -qq
        apt-get -o DPkg::Lock::Timeout=300 install -y -qq --no-install-recommends unzip
      fi
      URL="https://proxy.golang.org/golang.org/toolchain/@v/v0.0.1-go${GO_VERSION}.linux-${ARCH}.zip"
      printf 'Downloading Go %s from %s\n' "$GO_VERSION" "$URL" >&2
      # A failed connection stops this attempt. Only an explicit user retry
      # starts another download; progressing slow transfers keep their budget.
      if ! curl -fSL --connect-timeout 20 --speed-limit 1024 --speed-time 30 \
        -o "$GO_STAGE/download" "$URL"; then
        echo "Go download stopped: the connection failed or stalled. Retry the installation to try again." >&2
        exit 1
      fi
      echo "Verifying the downloaded Go toolchain" >&2
      if ! printf '%s  %s\n' "$MODULE_SHA" "$GO_STAGE/download" | sha256sum --check --status; then
        echo "Go download checksum mismatch. Installation stopped; retry to download a fresh copy." >&2
        exit 1
      fi
      mkdir -p "$GO_STAGE/unpack"
      unzip -q "$GO_STAGE/download" -d "$GO_STAGE/unpack"
      READY="$GO_STAGE/unpack/golang.org/toolchain@v0.0.1-go${GO_VERSION}.linux-${ARCH}"
      # Module ZIPs normalize file permissions; restore the executables.
      chmod 755 "$READY/bin/go" "$READY/bin/gofmt" "$READY/pkg/tool/linux_${ARCH}/"*
      "$READY/bin/go" version | grep -q "go${GO_VERSION} "
      echo "Installing the verified Go toolchain" >&2
      if [ -e "$GOROOT_DIR" ]; then mv "$GOROOT_DIR" "$GO_STAGE/previous"; fi
      mv "$READY" "$GOROOT_DIR"
    )
    install_application_go
  fi
`
