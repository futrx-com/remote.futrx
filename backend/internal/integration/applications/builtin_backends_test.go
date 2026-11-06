package applications

import (
	"context"
	"slices"
	"testing"
	"time"

	containerapplications "github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// Terminal and File Manager reach their project containers through host
// backend routes. This compiles each one through the generated module, exactly
// as the server does, and checks the routes it serves.
func TestBuiltInWorkspaceBackendsCompileAndDescribeTheirRoutes(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles backends with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	registry, err := containerapplications.NewRegistry(containerapplications.EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	host := New(sharedRoot(t), registry, Options{GoTool: testGoToolOverride()})
	t.Cleanup(host.Shutdown)

	for id, want := range map[string][]string{
		"terminal":     {"GET output", "POST input", "POST resize", "POST sessions"},
		"file-manager": {"GET archive", "GET list", "GET search"},
	} {
		t.Run(id, func(t *testing.T) {
			application, ok := registry.Get(id)
			if !ok || application.Backend == nil {
				t.Fatalf("%s has no backend", id)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			descriptor, err := host.Ensure(ctx, applications.Instance{
				ID:                 id + "-builtin",
				ApplicationID:      application.ID,
				ApplicationName:    application.Name,
				ApplicationVersion: application.Version,
				Scope:              string(svc.ScopeProject),
				ProjectID:          "p1",
				ContainerName:      "project-demo",
			})
			if err != nil {
				t.Fatalf("start %s: %v", id, err)
			}
			routes := make([]string, 0, len(descriptor.Routes))
			for _, route := range descriptor.Routes {
				routes = append(routes, route.Method+" "+route.Path)
			}
			slices.Sort(routes)
			if !slices.Equal(routes, want) {
				t.Fatalf("routes = %q, want %q", routes, want)
			}
		})
	}
}
