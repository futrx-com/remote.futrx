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
		"images/" + fixtureService + "/install.sh": file("#!/usr/bin/env bash\necho service\n"),

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
