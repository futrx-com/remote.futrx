package applications

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"
)

func minimalCatalog(manifest string) fstest.MapFS {
	return fstest.MapFS{
		"applications/example/application.json": {Data: []byte(manifest)},
	}
}

func TestLoadApplicationDiscoversDefaultInfrastructure(t *testing.T) {
	catalog := minimalCatalog(`{
		"name": "Example",
		"version": "1.0.0",
		"scopes": ["project"]
	}`)
	wantScript := []byte("#!/usr/bin/env bash\ntrue\n")
	catalog["applications/example/infra/install.sh"] = &fstest.MapFile{Data: wantScript}

	application, script, err := loadApplication(catalog, "example")
	if err != nil {
		t.Fatalf("load application: %v", err)
	}
	if application.Install != "infra/install.sh" {
		t.Errorf("install path = %q, want inferred default", application.Install)
	}
	if !bytes.Equal(script, wantScript) {
		t.Errorf("script = %q, want %q", script, wantScript)
	}
}

func TestLoadApplicationAllowsNoInfrastructure(t *testing.T) {
	catalog := minimalCatalog(`{
		"name": "Example",
		"version": "1.0.0",
		"scopes": ["project"]
	}`)
	catalog["applications/example/skills/example/SKILL.md"] = &fstest.MapFile{Data: []byte("# Example\n")}

	application, script, err := loadApplication(catalog, "example")
	if err != nil {
		t.Fatalf("load application: %v", err)
	}
	if application.Install != "" {
		t.Errorf("install path = %q, want none", application.Install)
	}
	if script != nil {
		t.Errorf("script = %q, want nil", script)
	}
}

func TestLoadApplicationGeneratesContainerInstallScript(t *testing.T) {
	catalog := minimalCatalog(`{
		"name": "Example",
		"version": "2.0.0",
		"scopes": ["project"]
	}`)
	catalog["applications/example/backend/container/cmd/example-agent/main.go"] = &fstest.MapFile{
		Data: []byte("package main\nfunc main() {}\n"),
	}

	application, script, err := loadApplication(catalog, "example")
	if err != nil {
		t.Fatalf("load application: %v", err)
	}
	if application.Container == nil || !application.NeedsContainer() {
		t.Fatalf("container capability = %+v", application.Container)
	}
	if application.Install != "" {
		t.Errorf("generated install unexpectedly claims an infra path: %q", application.Install)
	}
	if !strings.Contains(string(script), "APP_BUILD_VERSION='2.0.0+") ||
		!strings.Contains(string(script), "./cmd/example-agent") {
		t.Fatalf("generated install script is missing build metadata or command:\n%s", script)
	}
}

func TestLoadApplicationPrependsContainerBuildToCustomInstall(t *testing.T) {
	catalog := minimalCatalog(`{
		"name": "Example",
		"version": "1.0.0",
		"scopes": ["project"]
	}`)
	catalog["applications/example/backend/container/main.go"] = &fstest.MapFile{Data: []byte("package main\nfunc main() {}\n")}
	catalog["applications/example/infra/install.sh"] = &fstest.MapFile{Data: []byte("echo custom-install\n")}

	application, script, err := loadApplication(catalog, "example")
	if err != nil {
		t.Fatal(err)
	}
	if application.Install != defaultInstallScriptPath {
		t.Fatalf("install path = %q", application.Install)
	}
	generatedAt := bytes.Index(script, []byte("APP_BUILD_VERSION="))
	customAt := bytes.Index(script, []byte("echo custom-install"))
	if generatedAt < 0 || customAt < 0 || generatedAt >= customAt {
		t.Fatalf("container build was not prepended to custom install script")
	}
}

func TestHelloRemoteCombinesContainerBuildWithCustomProvisioning(t *testing.T) {
	registry, err := NewRegistry(EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	application, ok := registry.Get("hello-remote")
	if !ok {
		t.Fatal("hello-remote is missing from the embedded catalog")
	}
	if application.Install != defaultInstallScriptPath {
		t.Fatalf("install path = %q, want %q", application.Install, defaultInstallScriptPath)
	}
	script, ok := registry.Script(application.ID)
	if !ok {
		t.Fatal("hello-remote install program is missing")
	}
	generatedAt := bytes.Index(script, []byte("APP_BUILD_VERSION="))
	customAt := bytes.Index(script, []byte("provisioned-version"))
	serviceAt := bytes.Index(script, []byte("systemctl"))
	if generatedAt < 0 || customAt < 0 || generatedAt >= customAt || serviceAt >= 0 {
		t.Fatalf("container build must precede custom provisioning without owning systemd")
	}
}

func TestLoadApplicationRejectsInvalidInfrastructureOverrides(t *testing.T) {
	for _, tc := range []struct {
		name     string
		install  string
		wantText string
	}{
		{"outside infra", "install.sh", "install script must be inside infra/"},
		{"missing explicit script", "infra/custom.sh", `read install script "infra/custom.sh"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := minimalCatalog(`{
				"name": "Example",
				"version": "1.0.0",
				"scopes": ["project"],
				"install": "` + tc.install + `"
			}`)

			_, _, err := loadApplication(catalog, "example")
			if err == nil || !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("load error = %v, want text %q", err, tc.wantText)
			}
		})
	}
}
