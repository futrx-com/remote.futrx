// Package applicationtest supports app-owned integration tests against Remote's
// real catalog loader and builders. It contains no application-specific assertions.
package applicationtest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	hostapps "github.com/futrx-com/remote.futrx.com/internal/integration/applications"
	catalogapps "github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func Registry(t *testing.T) *catalogapps.Registry {
	t.Helper()
	registry, err := catalogapps.NewRegistry(catalogapps.EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

// BackendRoutes builds and starts the app through Remote's generated host
// module, then compares the advertised routes with the app-owned contract.
func BackendRoutes(t *testing.T, id string, want []string) {
	t.Helper()
	if testing.Short() {
		t.Skip("compiles a backend with the Go toolchain")
	}
	goTool := os.Getenv("REMOTE_APPLICATION_GO")
	if goTool == "" {
		goTool = "go"
	}
	resolved, err := exec.LookPath(goTool)
	if err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	goTool = resolved
	registry := Registry(t)
	app, ok := registry.Get(id)
	if !ok || app.Backend == nil {
		t.Fatalf("%s has no backend", id)
	}
	host := hostapps.New(t.TempDir(), registry, hostapps.Options{GoTool: goTool})
	t.Cleanup(host.Shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	descriptor, err := host.Ensure(ctx, applications.Instance{
		ID: id + "-builtin", ApplicationID: app.ID, ApplicationName: app.Name,
		ApplicationVersion: app.Version, Scope: "project", ProjectID: "p1", ContainerName: "project-demo",
	})
	if err != nil {
		t.Fatalf("start %s: %v", id, err)
	}
	var routes []string
	for _, route := range descriptor.Routes {
		routes = append(routes, route.Method+" "+route.Path)
	}
	slices.Sort(routes)
	if !slices.Equal(routes, want) {
		t.Fatalf("routes = %q, want %q", routes, want)
	}
}

// BuildContainer compiles the actual source payload from the public generated
// install script. It never executes provisioning commands from that script.
func BuildContainer(t *testing.T, script []byte, command string) {
	t.Helper()
	if testing.Short() {
		t.Skip("compiles the generated container module")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("Go toolchain is unavailable")
	}
	_, encoded, ok := strings.Cut(string(script), "base64 --decode <<'")
	if !ok {
		t.Fatal("generated install script has no source payload")
	}
	marker, encoded, ok := strings.Cut(encoded, "'")
	if !ok {
		t.Fatal("generated payload delimiter is missing")
	}
	_, encoded, _ = strings.Cut(encoded, "\n")
	encoded, _, ok = strings.Cut(encoded, "\n"+marker+"\n")
	if !ok {
		t.Fatal("generated payload is not terminated")
	}
	payload, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(encoded, "\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	directory := t.TempDir()
	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		relative := strings.TrimPrefix(header.Name, "infra/")
		if !filepath.IsLocal(relative) || header.Typeflag != tar.TypeReg {
			t.Fatalf("unexpected source payload member: %q", header.Name)
		}
		target := filepath.Join(directory, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, goTool, "build", "-mod=mod", "-buildvcs=false", "-o", filepath.Join(t.TempDir(), command), "./cmd/"+command)
	build.Dir = directory
	build.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("generated container module: %v\n%s", err, output)
	}
}
