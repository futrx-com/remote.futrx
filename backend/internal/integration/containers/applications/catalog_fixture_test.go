package applications

import (
	"testing"
	"testing/fstest"
)

// The catalog these tests load is written here rather than borrowed from the
// applications the server happens to ship. Every capability has a fixture, so
// the loader stays covered even when the shipped catalog changes.
const (
	fixtureService  = "fixture-service"
	fixturePortless = "fixture-portless"
	fixtureUI       = "fixture-ui"
	fixtureBackend  = "fixture-backend"
)

func fixtureCatalog() fstest.MapFS {
	file := func(data string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(data)} }
	return fstest.MapFS{
		"applications/" + fixtureService + "/application.json": file(`{
			"name": "Fixture Service",
			"version": "1.0.0",
			"scopes": ["global", "project"],
			"port": {"internal": 5432, "defaultExternal": 5432},
			"service": "fixture",
			"connection": {"user": "root", "passwordEnv": "FIXTURE_PASSWORD"}
		}`),
		"applications/" + fixtureService + "/infra/install.sh":    file("#!/usr/bin/env bash\necho service\n"),
		"applications/" + fixtureService + "/ui/scripts/main.js":  file("export default () => {}\n"),
		"applications/" + fixtureService + "/ui/style/panel.css":  file(".panel{}\n"),
		"applications/" + fixtureService + "/ui/views/popup.html": file("<p></p>\n"),

		// Portless infrastructure reaches a container without exposing anything
		// and needs a host binary it supplies itself.
		"applications/" + fixturePortless + "/application.json": file(`{
			"name": "Fixture Portless",
			"version": "2.1.0",
			"scopes": ["project"],
			"service": "fixture-portless",
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
		"applications/" + fixturePortless + "/infra/install.sh": file("#!/usr/bin/env bash\necho portless\n"),

		// A UI application declares its block explicitly rather than relying on the
		// layout convention, so both paths are exercised for real.
		"applications/" + fixtureUI + "/application.json": file(`{
			"name": "Fixture UI",
			"version": "0.4.0",
			"scopes": ["project"],
			"ui": {
				"entry": "scripts/main.js",
				"styles": ["style/panel.css"],
				"views": {"panel": "views/panel.html", "context": "views/context.html"}
			}
		}`),
		"applications/" + fixtureUI + "/ui/scripts/main.js":     file("import './selftest.js'\nexport default () => {}\n"),
		"applications/" + fixtureUI + "/ui/scripts/selftest.js": file("export const ok = true\n"),
		"applications/" + fixtureUI + "/ui/style/panel.css":     file(".panel{}\n"),
		"applications/" + fixtureUI + "/ui/views/panel.html":    file("<p></p>\n"),
		"applications/" + fixtureUI + "/ui/views/context.html":  file("<p></p>\n"),
		"applications/" + fixtureUI + "/ui/assets/logo.svg":     file("<svg/>\n"),

		"applications/" + fixtureBackend + "/application.json": file(`{
			"name": "Fixture Backend",
			"version": "3.0.0",
			"scopes": ["project"],
			"backend": {"access": "registered", "timeoutMs": 10000}
		}`),
		"applications/" + fixtureBackend + "/backend/main.go": file("package main\n\nfunc main() {}\n"),
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
