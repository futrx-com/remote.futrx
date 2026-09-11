package applications

import (
	"testing"
	"testing/fstest"
)

// The catalog these tests load is written here rather than borrowed from the
// images the server happens to ship. Every kind, and every optional block,
// has a fixture — so the catalog loader stays covered no matter which installable
// images exist, and removing an image from the shipped catalog (or moving one
// out into a separately distributed package) cannot quietly delete a test.
const (
	fixtureService = "fixture-service"
	fixtureTool    = "fixture-tool"
	fixtureUI      = "fixture-ui"
	fixtureBackend = "fixture-backend"
)

func fixtureCatalog() fstest.MapFS {
	file := func(data string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(data)} }
	return fstest.MapFS{
		"images/" + fixtureService + "/image.json": file(`{
			"name": "Fixture Service",
			"version": "1.0.0",
			"scopes": ["global", "project"],
			"port": {"internal": 5432, "defaultExternal": 5432},
			"service": "fixture",
			"connection": {"user": "root", "passwordEnv": "FIXTURE_PASSWORD"}
		}`),
		"images/" + fixtureService + "/install.sh":          file("#!/usr/bin/env bash\necho service\n"),
		"images/" + fixtureService + "/ui/scripts/main.js":  file("export default () => {}\n"),
		"images/" + fixtureService + "/ui/style/panel.css":  file(".panel{}\n"),
		"images/" + fixtureService + "/ui/views/popup.html": file("<p></p>\n"),

		// A tool reaches a container without exposing anything, needs a host
		// binary it supplies itself.
		"images/" + fixtureTool + "/image.json": file(`{
			"name": "Fixture Tool",
			"version": "2.1.0",
			"type": "tool",
			"scopes": ["project"],
			"service": "fixture-tool",
			"hostTools": [{
				"name": "fixture-backup",
				"version": "1.2.3",
				"downloads": {
					"amd64": {
						"url": "https://example.invalid/fixture-backup-amd64",
						"sha256": "0000000000000000000000000000000000000000000000000000000000000000"
					},
					"arm64": {
						"url": "https://example.invalid/fixture-backup-arm64",
						"sha256": "1111111111111111111111111111111111111111111111111111111111111111"
					}
				}
			}]
		}`),
		"images/" + fixtureTool + "/install.sh": file("#!/usr/bin/env bash\necho tool\n"),

		// A UI image declares its block explicitly rather than relying on the
		// layout convention, so both paths are exercised for real.
		"images/" + fixtureUI + "/image.json": file(`{
			"name": "Fixture UI",
			"version": "0.4.0",
			"type": "ui",
			"scopes": ["project"],
			"ui": {
				"entry": "scripts/main.js",
				"styles": ["style/panel.css"],
				"views": {"panel": "views/panel.html", "context": "views/context.html"}
			}
		}`),
		"images/" + fixtureUI + "/ui/scripts/main.js":     file("import './selftest.js'\nexport default () => {}\n"),
		"images/" + fixtureUI + "/ui/scripts/selftest.js": file("export const ok = true\n"),
		"images/" + fixtureUI + "/ui/style/panel.css":     file(".panel{}\n"),
		"images/" + fixtureUI + "/ui/views/panel.html":    file("<p></p>\n"),
		"images/" + fixtureUI + "/ui/views/context.html":  file("<p></p>\n"),
		"images/" + fixtureUI + "/ui/assets/logo.svg":     file("<svg/>\n"),

		"images/" + fixtureBackend + "/image.json": file(`{
			"name": "Fixture Backend",
			"version": "3.0.0",
			"type": "backend",
			"scopes": ["project"],
			"backend": {"access": "registered", "timeoutMs": 10000}
		}`),
		"images/" + fixtureBackend + "/plugin/main.go": file("package main\n\nfunc main() {}\n"),
	}
}

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistryFromFS(fixtureCatalog())
	if err != nil {
		t.Fatalf("load fixture catalog: %v", err)
	}
	return r
}
