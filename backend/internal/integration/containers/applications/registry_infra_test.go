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
