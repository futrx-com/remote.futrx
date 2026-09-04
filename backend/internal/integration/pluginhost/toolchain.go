// Package pluginhost compiles the Go source an installable image ships in its
// plugin/ directory and runs the result as a child process, forwarding calls
// to it over hashicorp/go-plugin.
//
// It is the integration half of the backend-plugin feature: everything that
// touches the Go toolchain, the filesystem, and a process lives here, so the
// service layer decides only who may call a plugin and when one should be
// running.
package pluginhost

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
)

// goToolCandidates are the places a Go toolchain is found on a server built by
// infra/, in preference order. The installer puts Go in /usr/local/go; PATH is
// tried first so a developer's own toolchain wins during local work.
var goToolCandidates = []string{
	"/usr/local/go/bin/go",
	"/usr/lib/go/bin/go",
}

// findGoTool locates the Go toolchain used to compile plugins. A server
// without one can still run everything else, so the error is reported to the
// instance that needed it rather than failing startup.
func findGoTool(configured string) (string, error) {
	if configured = strings.TrimSpace(configured); configured != "" {
		if _, err := os.Stat(configured); err == nil {
			return configured, nil
		}
		return "", fmt.Errorf("REMOTE_PLUGIN_GO=%s is not executable", configured)
	}
	if path, err := exec.LookPath("go"); err == nil {
		return path, nil
	}
	for _, candidate := range goToolCandidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf(
		"no Go toolchain found: install Go, or set REMOTE_PLUGIN_GO to its path")
}

// modulePins is the exact set of module versions the server itself was built
// with. Generating a plugin's go.mod from it is what lets a plugin build
// offline: minimal version selection then resolves to modules the server's own
// build already put in the module cache, instead of picking older ones that
// would have to be downloaded.
type modulePins struct {
	// goVersion is the "go" directive, e.g. "1.25.13". Taken from the running
	// binary so a plugin is compiled by the same toolchain as its host.
	goVersion string
	// requires maps module path to version, excluding the server's own module.
	requires map[string]string
}

func readModulePins() modulePins {
	pins := modulePins{
		goVersion: strings.TrimPrefix(runtime.Version(), "go"),
		requires:  map[string]string{},
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return pins
	}
	for _, dep := range info.Deps {
		for dep.Replace != nil {
			dep = dep.Replace
		}
		if dep.Path == "" || dep.Version == "" || dep.Version == "(devel)" {
			continue
		}
		pins.requires[dep.Path] = dep.Version
	}
	return pins
}

// require returns the pinned version of a module, or the fallback when the
// build info did not carry one — which happens under `go test`, where the
// dependency is present but not recorded as the test binary's.
func (p modulePins) require(path, fallback string) string {
	if version, ok := p.requires[path]; ok {
		return version
	}
	return fallback
}

// indirectBlock renders every pinned module except the ones listed as direct,
// sorted, so a generated go.mod is byte-stable for a given server build. That
// stability matters: the file is part of a plugin's build fingerprint.
func (p modulePins) indirectBlock(direct ...string) string {
	skip := map[string]bool{}
	for _, path := range direct {
		skip[path] = true
	}
	paths := make([]string, 0, len(p.requires))
	for path := range p.requires {
		if !skip[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	var builder strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&builder, "\t%s %s // indirect\n", path, p.requires[path])
	}
	return builder.String()
}

// goEnv builds the environment a plugin build runs in. GOCACHE is pinned under
// the work directory because the server may run as a system user with no
// writable HOME, and a build that cannot cache is a build that fails.
func goEnv(workRoot string, offline bool) []string {
	env := append(os.Environ(),
		"GOFLAGS=-mod=mod",
		"GOCACHE="+filepath.Join(workRoot, "build-cache"),
	)
	if strings.TrimSpace(os.Getenv("HOME")) == "" {
		env = append(env, "HOME="+filepath.Join(workRoot, "home"))
	}
	if offline {
		// The first attempt refuses the network: the server's own build
		// already populated the module cache with every version the generated
		// go.mod pins, so a successful offline build is the normal path and a
		// fast one.
		env = append(env, "GOPROXY=off")
	}
	return env
}
