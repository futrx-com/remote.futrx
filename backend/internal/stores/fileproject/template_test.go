package fileproject

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// writeLegacyMeta drops a meta.json with an explicit field set, bypassing the
// store so the fixture is exactly what an older release wrote.
func writeLegacyMeta(t *testing.T, dataDir, id, body string) {
	t.Helper()
	dir := filepath.Join(dataDir, "projects", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadsProjectMetadataWrittenBeforeTemplatesExisted(t *testing.T) {
	dataDir := t.TempDir()
	writeLegacyMeta(t, dataDir, "abcdef123456", `{
  "id": "abcdef123456",
  "name": "Legacy Project",
  "slug": "legacy-project",
  "cwd": "/var/lib/remote/projects/legacy-project/workspace",
  "containerName": "legacy-project",
  "status": "running",
  "createdAt": 1700000000000,
  "updatedAt": 1700000000000
}`)

	store, err := NewWithWorkspaceRoot(dataDir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	meta, err := store.Get(context.Background(), serviceproject.ID("abcdef123456"))
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if meta.Template != "" {
		t.Fatalf("Template = %q, want it absent for legacy metadata", meta.Template)
	}
	if got := meta.TemplateName(); got != serviceproject.DefaultTemplate {
		t.Fatalf("TemplateName() = %q, want %q", got, serviceproject.DefaultTemplate)
	}
	if meta.Slug != "legacy-project" || meta.Status != serviceproject.StatusRunning {
		t.Fatalf("legacy metadata was not read faithfully: %#v", meta)
	}

	// The slug index must have picked the legacy project up as well.
	if _, err := store.GetBySlug(context.Background(), "legacy-project"); err != nil {
		t.Fatalf("GetBySlug() = %v", err)
	}
}

func TestTemplateSurvivesUpdatesAndIsOmittedWhenEmpty(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewWithWorkspaceRoot(dataDir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	templated, err := store.Create(ctx, serviceproject.Meta{
		Name:     "Shop",
		Slug:     serviceproject.Slugify("Shop"),
		Status:   serviceproject.StatusProvisioning,
		Template: "wordpress",
	})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := store.Create(ctx, serviceproject.Meta{
		Name:   "Plain",
		Slug:   serviceproject.Slugify("Plain"),
		Status: serviceproject.StatusProvisioning,
	})
	if err != nil {
		t.Fatal(err)
	}

	renamed, err := store.Update(ctx, templated.ID, func(m *serviceproject.Meta) {
		m.Name = "Shop v2"
	})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Template != "wordpress" {
		t.Fatalf("Template = %q, want it preserved across an update", renamed.Template)
	}

	statused, err := store.SetStatus(ctx, templated.ID, serviceproject.StatusRunning, "")
	if err != nil {
		t.Fatal(err)
	}
	if statused.Template != "wordpress" {
		t.Fatalf("Template = %q, want it preserved across a status change", statused.Template)
	}

	// A project on the default template writes no field at all, so metadata
	// stays byte-compatible with what older releases produced.
	raw, err := os.ReadFile(filepath.Join(dataDir, "projects", string(plain.ID), "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["template"]; present {
		t.Fatalf("meta.json for a default-template project carries %q: %s", "template", raw)
	}
}
