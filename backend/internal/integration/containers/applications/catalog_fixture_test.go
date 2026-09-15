package applications

import (
	"testing"
	"testing/fstest"
)

// The catalog these tests load is written here rather than borrowed from the
// applications the server happens to ship. Its exposed and portless entries
// keep both infrastructure shapes covered even when the shipped catalog changes.
const (
	fixtureService  = "fixture-service"
	fixturePortless = "fixture-portless"
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
		"applications/" + fixtureService + "/infra/install.sh": file("#!/usr/bin/env bash\necho service\n"),

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
