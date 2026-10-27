package provisioning

import (
	"bufio"
	_ "embed"
	"regexp"
	"strings"
)

// versions.env is the repo's single version manifest (infra/versions.env is a
// symlink to it). Embedding keeps container provisioning self-contained: the
// backend and cmd/build-base-image need no file on disk at runtime.
//
//go:embed versions.env
var versionManifest string

var (
	semanticVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	// Pins that are not semver: major streams ("22"), four-part Chrome
	// versions, sha256 hex, release tags, owner/repo slugs.
	pinPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

func manifestValue(key string) (string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(versionManifest))
	for scanner.Scan() {
		name, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if ok && name == key {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

// MustCLIVersion returns the pinned semver for an agent CLI, panicking on a
// missing or malformed entry so a bad manifest fails at startup, not
// mid-provision.
func MustCLIVersion(key string) string {
	value, ok := manifestValue(key)
	if !ok {
		panic("missing " + key + " in versions.env")
	}
	if !ValidCLIVersion(value) {
		panic("invalid " + key + " in versions.env")
	}
	return value
}

// ValidCLIVersion reports whether a provider pin uses the strict semver shape
// supported by shared host and container version checks.
func ValidCLIVersion(value string) bool {
	return semanticVersionPattern.MatchString(value)
}

// MustPin returns any pinned value from the manifest (versions that are not
// strict semver: Node major, Chrome-for-Testing builds, sha256 pins, release
// tags). Same fail-fast contract as MustCLIVersion.
func MustPin(key string) string {
	value, ok := manifestValue(key)
	if !ok {
		panic("missing " + key + " in versions.env")
	}
	if !pinPattern.MatchString(value) {
		panic("invalid " + key + " in versions.env")
	}
	return value
}
